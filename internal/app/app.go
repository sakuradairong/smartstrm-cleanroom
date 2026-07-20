package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/history"
	renamer "github.com/sakuradairong/smartstrm-cleanroom/internal/rename"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/task"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/tmdb"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

type App struct {
	config   config.Config
	storages map[string]storage.Storage
	manager  *task.Manager
	tmdb     *tmdb.Client
	history  *history.Store
	uiHTML   string
	configMu sync.Mutex
	handler  http.Handler
}

func New(cfg config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	storages, err := storage.BuildAll(cfg.Storages)
	if err != nil {
		return nil, err
	}
	for _, taskConfig := range cfg.Tasks {
		if taskConfig.FileIDMode {
			if _, ok := storages[taskConfig.StorageID].(storage.FileIDStorage); !ok {
				return nil, fmt.Errorf("task %q enables file_id_mode but storage %q does not support stable file IDs", taskConfig.ID, taskConfig.StorageID)
			}
		}
	}
	manager := task.NewManager(cfg.Tasks, storages, task.NewGenerator(cfg.PublicURL, cfg.WebhookToken, cfg.Plugins...))
	historyPath := cfg.History.Path
	if historyPath == "" && cfg.Path != "" {
		historyPath = filepath.Join(filepath.Dir(cfg.Path), "history.jsonl")
	}
	historyStore, err := history.New(historyPath, cfg.History.MaxEntries)
	if err != nil {
		return nil, fmt.Errorf("initialize history: %w", err)
	}
	manager.SetHistory(historyStore)
	tmdbClient, err := tmdb.New(cfg.TMDB)
	if err != nil {
		return nil, err
	}
	uiHTML, err := renderIndexHTML(strings.TrimSpace(cfg.TMDB.APIKey) != "" && cfg.TMDB.APIKey != config.RedactedSecret)
	if err != nil {
		return nil, err
	}
	application := &App{config: cfg, storages: storages, manager: manager, tmdb: tmdbClient, history: historyStore, uiHTML: uiHTML}
	application.handler = application.routes()
	return application, nil
}

func (a *App) Start(ctx context.Context) error { return a.manager.Start(ctx) }
func (a *App) Handler() http.Handler           { return a.handler }

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", a.basicAuth(http.HandlerFunc(a.home)))
	mux.HandleFunc("GET /robots.txt", robots)
	mux.HandleFunc("GET /favicon.ico", emptyFavicon)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /api/tasks", a.basicAuth(http.HandlerFunc(a.listTasks)))
	mux.Handle("POST /api/tasks/{id}/run", a.basicAuth(http.HandlerFunc(a.runTask)))
	mux.Handle("POST /api/tasks/{id}/stop", a.basicAuth(http.HandlerFunc(a.stopTask)))
	mux.Handle("POST /api/tasks/{id}/full-overwrite", a.basicAuth(http.HandlerFunc(a.fullOverwriteTask)))
	mux.Handle("POST /api/tasks/{id}/replace-content", a.basicAuth(http.HandlerFunc(a.replaceTaskContent)))
	mux.Handle("DELETE /api/tasks/{id}/generated", a.basicAuth(http.HandlerFunc(a.clearTaskGenerated)))
	mux.Handle("POST /api/tasks/{id}/covers", a.basicAuth(http.HandlerFunc(a.extractTaskCovers)))
	mux.Handle("GET /api/tasks/{id}/preview", a.basicAuth(http.HandlerFunc(a.previewTask)))
	mux.Handle("GET /api/storages", a.basicAuth(http.HandlerFunc(a.listStorages)))
	mux.Handle("GET /api/storages/{id}/entries", a.basicAuth(http.HandlerFunc(a.listEntries)))
	mux.Handle("GET /api/storages/{id}/matching-tasks", a.basicAuth(http.HandlerFunc(a.matchingTasks)))
	mux.Handle("GET /api/storages/{id}/stream-url", a.basicAuth(http.HandlerFunc(a.getStreamURL)))
	mux.Handle("POST /api/storages/{id}/mkdir", a.basicAuth(http.HandlerFunc(a.mkdirEntry)))
	mux.Handle("POST /api/storages/{id}/rename", a.basicAuth(http.HandlerFunc(a.renameEntry)))
	mux.Handle("POST /api/storages/{id}/move", a.basicAuth(http.HandlerFunc(a.moveEntry)))
	mux.Handle("POST /api/storages/{id}/batch-rename", a.basicAuth(http.HandlerFunc(a.batchRename)))
	mux.Handle("DELETE /api/storages/{id}/entry", a.basicAuth(http.HandlerFunc(a.deleteEntry)))
	mux.Handle("GET /api/tmdb/search", a.basicAuth(http.HandlerFunc(a.searchTMDB)))
	mux.Handle("GET /api/tmdb/poster", a.basicAuth(http.HandlerFunc(a.tmdbPoster)))
	mux.Handle("GET /api/tmdb/{type}/{id}", a.basicAuth(http.HandlerFunc(a.tmdbDetails)))
	mux.Handle("POST /api/storages/{id}/tmdb-rename", a.basicAuth(http.HandlerFunc(a.tmdbRename)))
	mux.Handle("GET /api/config", a.basicAuth(http.HandlerFunc(a.getConfig)))
	mux.Handle("PUT /api/config", a.basicAuth(http.HandlerFunc(a.putConfig)))
	mux.Handle("POST /api/config/restore", a.basicAuth(http.HandlerFunc(a.restoreConfig)))
	mux.Handle("POST /api/config/webhook-token", a.basicAuth(http.HandlerFunc(a.resetWebhookToken)))
	mux.Handle("GET /api/history", a.basicAuth(http.HandlerFunc(a.listHistory)))
	mux.Handle("GET /api/history/stream", a.basicAuth(http.HandlerFunc(a.streamHistory)))
	mux.Handle("POST /webhook/run", webhookCORS(http.HandlerFunc(a.webhookRun)))
	mux.Handle("OPTIONS /webhook/run", webhookCORS(http.HandlerFunc(webhookOptions)))
	mux.Handle("POST /webhook/emby", webhookCORS(http.HandlerFunc(a.embyDelete)))
	mux.Handle("OPTIONS /webhook/emby", webhookCORS(http.HandlerFunc(webhookOptions)))
	mux.Handle("POST /webhook/{token}", webhookCORS(http.HandlerFunc(a.publicWebhook)))
	mux.Handle("OPTIONS /webhook/{token}", webhookCORS(http.HandlerFunc(webhookOptions)))
	mux.Handle("POST /webhook/{token}/file_notify", webhookCORS(http.HandlerFunc(a.cloudDrive2Webhook)))
	mux.Handle("OPTIONS /webhook/{token}/file_notify", webhookCORS(http.HandlerFunc(webhookOptions)))
	mux.Handle("POST /webhook/{token}/moviepilot", webhookCORS(http.HandlerFunc(a.moviePilotWebhook)))
	mux.Handle("OPTIONS /webhook/{token}/moviepilot", webhookCORS(http.HandlerFunc(webhookOptions)))
	mux.HandleFunc("GET /stream/{storage}", a.stream)
	return requestLog(mux)
}

func (a *App) home(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, a.uiHTML)
}

func renderIndexHTML(tmdbConfigured bool) (string, error) {
	buttonAnchor := `<button onclick="tmdbRecognize()">影视识别</button>`
	functionAnchor := `async function tmdbRecognize(){const query=prompt('影视名称');`
	filterAnchor := `<input id="filterInput" placeholder="当前目录筛选" oninput="renderEntries()">`
	browseAnchor := `<button onclick="browse(pathInput.value)">前往</button>`
	renderAnchor := `renderEntries=function(){const q=filterInput.value.toLowerCase(),items=currentEntries.filter(x=>x.name.toLowerCase().includes(q)).slice().sort(compareStorageEntries);`
	rowAnchor := `return '<tr><td>'+label`
	errorResultAnchor := `result=x.error||(`
	resultAnchor := `+' / 删除 '+(x.result?.removed||0))`
	historyErrorAnchor := `esc(e.error||(`
	historyResultAnchor := `+' / 跳过 '+(e.skipped||0)))`
	historyEventAnchor := `if(!historyEvents.some(x=>x.id===item.id)){historyEvents.unshift(item);historyEvents=historyEvents.slice(0,200);renderHistory()}`
	historyLoadAnchor := `async function loadHistory(){`
	styleAnchor := `#storageTable td:last-child{white-space:nowrap}</style>`
	sectionAnchor := `</table></div></section>
<section id="configView"`
	scriptAnchor := `</script></html>`
	pollAnchor := `load();setInterval(load,3000)`
	if strings.Count(indexHTML, buttonAnchor) != 1 || strings.Count(indexHTML, functionAnchor) != 1 || strings.Count(indexHTML, filterAnchor) != 1 || strings.Count(indexHTML, browseAnchor) != 1 || strings.Count(indexHTML, renderAnchor) != 1 || strings.Count(indexHTML, rowAnchor) != 1 || strings.Count(indexHTML, errorResultAnchor) != 1 || strings.Count(indexHTML, resultAnchor) != 1 || strings.Count(indexHTML, historyErrorAnchor) != 1 || strings.Count(indexHTML, historyResultAnchor) != 1 || strings.Count(indexHTML, historyEventAnchor) != 1 || strings.Count(indexHTML, historyLoadAnchor) != 1 || strings.Count(indexHTML, styleAnchor) != 1 || strings.Count(indexHTML, sectionAnchor) != 1 || strings.Count(indexHTML, scriptAnchor) != 1 || strings.Count(indexHTML, pollAnchor) != 2 {
		return "", fmt.Errorf("management UI injection anchors are invalid")
	}
	status := ""
	if !tmdbConfigured {
		status = "TMDB API Key 未配置，请在配置页设置后重启服务"
	}
	result := strings.Replace(indexHTML, buttonAnchor, buttonAnchor+`<span id="tmdbStatus" class="muted">`+status+`</span>`, 1)
	preflight := "const tmdbConfigured=" + strconv.FormatBool(tmdbConfigured) + `;async function tmdbRecognize(){if(!tmdbConfigured){alert('TMDB API Key 未配置，请在配置页设置后重启服务');return}const query=prompt('影视名称');`
	result = strings.Replace(result, functionAnchor, preflight, 1)
	result = strings.Replace(result, filterAnchor, filterAnchor+`<span id="storageCount" class="muted" role="status" aria-live="polite"></span>`, 1)
	result = strings.Replace(result, browseAnchor, browseAnchor+`<button id="runCurrentButton" class="hidden" onclick="runCurrentDirectory()">运行当前</button>`, 1)
	countUpdate := renderAnchor + `storageCount.textContent='显示 '+items.length+' / 共 '+currentEntries.length+' 项';`
	result = strings.Replace(result, renderAnchor, countUpdate, 1)
	result = strings.Replace(result, rowAnchor, `return '<tr'+(storageIsMedia(x)?' tabindex="0" aria-haspopup="menu" data-entry-index="'+i+'"':'')+'><td>'+label`, 1)
	result = strings.Replace(result, errorResultAnchor, `result=x.error?x.error+' / 失败 '+(x.result?.failed||0):(`, 1)
	result = strings.Replace(result, resultAnchor, `+' / 删除 '+(x.result?.removed||0)+' / 失败 '+(x.result?.failed||0))`, 1)
	result = strings.Replace(result, historyErrorAnchor, `esc(e.error?e.error+' / 失败 '+(e.failed||0):(`, 1)
	result = strings.Replace(result, historyResultAnchor, `+' / 跳过 '+(e.skipped||0)+' / 失败 '+(e.failed||0)))`, 1)
	result = strings.Replace(result, historyEventAnchor, `queueHistoryEvent(item)`, 1)
	result = strings.Replace(result, historyLoadAnchor, `async function loadHistory(){clearPendingHistoryEvents();`, 1)
	menuCSS := `#storageTable td:last-child{white-space:nowrap}.storage-menu{position:fixed;z-index:1000;min-width:170px;padding:.35rem;background:#202832;border:1px solid #596675;border-radius:8px;box-shadow:0 8px 24px #0008}.storage-menu button{display:block;width:100%;text-align:left;background:transparent}.storage-menu button:hover,.storage-menu button:focus{background:#1685f8}</style>`
	result = strings.Replace(result, styleAnchor, menuCSS, 1)
	menuHTML := `</table></div><div id="storageContextMenu" class="storage-menu hidden" role="menu" aria-label="文件操作"><button id="storageContextCopy" role="menuitem">复制STRM地址</button></div></section>
<section id="configView"`
	result = strings.Replace(result, sectionAnchor, menuHTML, 1)
	result = strings.ReplaceAll(result, pollAnchor, "")
	menuJS := `let historyPending=[],historyPendingIDs=new Set(),historyRenderFrame=0;function clearPendingHistoryEvents(){if(historyRenderFrame)cancelAnimationFrame(historyRenderFrame);historyRenderFrame=0;historyPending=[];historyPendingIDs.clear()}function queueHistoryEvent(item){if(historyPendingIDs.has(item.id)||historyEvents.some(x=>x.id===item.id))return;if(historyPending.length===200){historyPendingIDs.delete(historyPending.shift().id)}historyPending.push(item);historyPendingIDs.add(item.id);if(!historyRenderFrame)historyRenderFrame=requestAnimationFrame(flushHistoryEvents)}function flushHistoryEvents(){historyRenderFrame=0;if(!historyPending.length)return;const known=new Set(historyEvents.map(x=>x.id));for(const item of historyPending){if(!known.has(item.id)){historyEvents.unshift(item);known.add(item.id)}}historyPending=[];historyPendingIDs.clear();historyEvents=historyEvents.slice(0,200);renderHistory()}let taskPollTimer=0,taskPollRunning=false;async function taskPollCycle(){if(taskPollRunning||document.hidden)return;taskPollRunning=true;try{await load()}finally{taskPollRunning=false}if(!document.hidden){clearTimeout(taskPollTimer);taskPollTimer=setTimeout(taskPollCycle,3000)}}document.addEventListener('visibilitychange',()=>{if(document.hidden)clearTimeout(taskPollTimer);else taskPollCycle()});taskPollCycle();let currentMatchingTasks=[];async function loadCurrentTasks(remotePath,storageID){runCurrentButton.classList.add('hidden');currentMatchingTasks=[];try{const matches=await api('/api/storages/'+encodeURIComponent(storageID)+'/matching-tasks?path='+encodeURIComponent(remotePath));if(pathInput.value!==remotePath||storageSelect.value!==storageID)return;currentMatchingTasks=matches;runCurrentButton.classList.toggle('hidden',!matches.length)}catch{runCurrentButton.classList.add('hidden')}}const browseEntries=browse;browse=async function(path){const remotePath=path||'/',storageID=storageSelect.value;await browseEntries(remotePath);if(pathInput.value===remotePath&&storageSelect.value===storageID)await loadCurrentTasks(remotePath,storageID)};async function runCurrentDirectory(){const task=currentMatchingTasks[0];if(!task)return;const remotePath=pathInput.value;try{await api('/api/tasks/'+encodeURIComponent(task.id)+'/run',{method:'POST',...body({path:remotePath})});alert('已将 '+(task.name||task.id)+' 的当前目录加入队列')}catch(e){alert(e.message)}}function storageIsMedia(item){return storageFileIcon(item)==='🎬'}let storageContextIndex=-1;function closeStorageContextMenu(){storageContextMenu.classList.add('hidden');storageContextIndex=-1}function openStorageContextMenu(event,row){closeStorageContextMenu();if(!row)return;const index=Number(row.dataset.entryIndex),item=currentEntries[index];if(!item||!storageIsMedia(item))return;event.preventDefault();storageContextIndex=index;storageContextMenu.classList.remove('hidden');const width=storageContextMenu.offsetWidth,height=storageContextMenu.offsetHeight,rect=row.getBoundingClientRect(),keyboard=event.type==='keydown',x=keyboard?rect.left+20:event.clientX,y=keyboard?rect.top+20:event.clientY;storageContextMenu.style.left=Math.max(8,Math.min(x,innerWidth-width-8))+'px';storageContextMenu.style.top=Math.max(8,Math.min(y,innerHeight-height-8))+'px';storageContextCopy.focus()}entries.addEventListener('contextmenu',event=>openStorageContextMenu(event,event.target.closest('tr')));entries.addEventListener('keydown',event=>{if(event.key==='ContextMenu'||(event.shiftKey&&event.key==='F10'))openStorageContextMenu(event,event.target.closest('tr'))});storageContextCopy.addEventListener('click',async()=>{const index=storageContextIndex;closeStorageContextMenu();if(index>=0)await copyURL(index)});document.addEventListener('pointerdown',event=>{if(!storageContextMenu.contains(event.target))closeStorageContextMenu()});document.addEventListener('keydown',event=>{if(event.key==='Escape')closeStorageContextMenu()});` + scriptAnchor
	return strings.Replace(result, scriptAnchor, menuJS, 1), nil
}

func robots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.WriteString(w, "User-agent: *\nDisallow: /\n")
}

func emptyFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listTasks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tasks": a.manager.Configs(), "statuses": a.manager.Statuses()})
}

func (a *App) runTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	if err := a.manager.Enqueue(r.PathValue("id"), body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (a *App) stopTask(w http.ResponseWriter, r *http.Request) {
	if err := a.manager.Stop(r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
}

func (a *App) fullOverwriteTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}
	if err := a.manager.FullOverwrite(r.PathValue("id"), body.Path); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (a *App) replaceTaskContent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Preview bool   `json:"preview"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := a.manager.ReplaceContent(r.PathValue("id"), body.From, body.To, body.Preview)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) clearTaskGenerated(w http.ResponseWriter, r *http.Request) {
	removed, err := a.manager.Clear(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (a *App) extractTaskCovers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path      string `json:"path"`
		Position  string `json:"position"`
		Overwrite bool   `json:"overwrite"`
		Preview   bool   `json:"preview"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	position := 10 * time.Second
	if body.Position != "" {
		parsed, err := time.ParseDuration(body.Position)
		if err != nil || parsed < 0 || parsed > 24*time.Hour {
			writeError(w, http.StatusBadRequest, fmt.Errorf("position must be a duration between 0 and 24h"))
			return
		}
		position = parsed
	}
	timeout := time.Duration(a.config.MediaTools.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	result, err := a.manager.ExtractCovers(r.Context(), r.PathValue("id"), task.CoverOptions{Binary: a.config.MediaTools.FFmpeg, PublicURL: a.config.PublicURL, Subpath: body.Path, Position: position, Timeout: timeout, Overwrite: body.Overwrite, Preview: body.Preview})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) previewTask(w http.ResponseWriter, r *http.Request) {
	preview, err := a.manager.Preview(r.Context(), r.PathValue("id"), r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

type storageSummary struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Root string `json:"root"`
}

func (a *App) listStorages(w http.ResponseWriter, _ *http.Request) {
	result := make([]storageSummary, 0, len(a.config.Storages))
	for _, item := range a.config.Storages {
		result = append(result, storageSummary{ID: item.ID, Type: item.Type, Root: item.Root})
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) listEntries(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	entries, err := instance.List(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

type matchingTaskSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

func (a *App) matchingTasks(w http.ResponseWriter, r *http.Request) {
	storageID := r.PathValue("id")
	if _, ok := a.storage(storageID, w); !ok {
		return
	}
	remotePath := r.URL.Query().Get("path")
	if len(remotePath) > 4096 || strings.ContainsRune(remotePath, '\x00') {
		writeError(w, http.StatusBadRequest, fmt.Errorf("storage path is invalid"))
		return
	}
	result := make([]matchingTaskSummary, 0)
	for _, id := range a.manager.MatchTasks(storageID, remotePath) {
		_, cfg, err := a.manager.ResolveTask(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		result = append(result, matchingTaskSummary{ID: id, Name: cfg.Name, Source: storage.CleanRemote(cfg.Source)})
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) getStreamURL(w http.ResponseWriter, r *http.Request) {
	storageID := r.PathValue("id")
	instance, ok := a.storage(storageID, w)
	if !ok {
		return
	}
	remotePath := storage.CleanRemoteExact(r.URL.Query().Get("path"))
	if r.URL.Query().Get("file_id") == "true" {
		fileIDSource, supported := instance.(storage.FileIDStorage)
		if !supported {
			writeError(w, http.StatusBadRequest, fmt.Errorf("storage does not support stable file IDs"))
			return
		}
		fileID, err := fileIDSource.FileID(r.Context(), remotePath)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		signed := signature.Create(a.config.WebhookToken, storageID, "id:"+fileID)
		streamURL := strings.TrimRight(a.config.PublicURL, "/") + "/stream/" + url.PathEscape(storageID) + "?id=" + url.QueryEscape(fileID) + "&sig=" + url.QueryEscape(signed)
		writeJSON(w, http.StatusOK, map[string]string{"url": streamURL})
		return
	}
	signed := signature.Create(a.config.WebhookToken, storageID, remotePath)
	streamURL := strings.TrimRight(a.config.PublicURL, "/") + "/stream/" + url.PathEscape(storageID) + "?path=" + url.QueryEscape(remotePath) + "&sig=" + url.QueryEscape(signed)
	writeJSON(w, http.StatusOK, map[string]string{"url": streamURL})
}

func (a *App) mkdirEntry(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := instance.Mkdir(r.Context(), body.Path); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (a *App) renameEntry(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	var body struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := instance.Rename(r.Context(), body.Path, body.NewName); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

func (a *App) moveEntry(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	var body struct {
		Path        string `json:"path"`
		Destination string `json:"destination"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := instance.Move(r.Context(), body.Path, body.Destination); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

func (a *App) batchRename(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	var body struct {
		Path              string `json:"path"`
		Mode              string `json:"mode"`
		Pattern           string `json:"pattern"`
		Replacement       string `json:"replacement"`
		Template          string `json:"template"`
		Prefix            string `json:"prefix"`
		Suffix            string `json:"suffix"`
		Start             int    `json:"start"`
		Width             int    `json:"width"`
		PreserveExtension bool   `json:"preserve_extension"`
		Preview           bool   `json:"preview"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	directory := storage.CleanRemote(body.Path)
	entries, err := instance.List(r.Context(), directory)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	changes, err := renamer.Plan(entries, renamer.Options{
		Mode: body.Mode, Pattern: body.Pattern, Replacement: body.Replacement, Template: body.Template,
		Prefix: body.Prefix, Suffix: body.Suffix, Start: body.Start, Width: body.Width, PreserveExtension: body.PreserveExtension,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !body.Preview {
		if err := renamer.Execute(r.Context(), instance, directory, changes); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": body.Preview, "changes": changes})
}

func (a *App) deleteEntry(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	if err := instance.Delete(r.Context(), r.URL.Query().Get("path")); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) searchTMDB(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	results, err := a.tmdb.Search(r.Context(), r.URL.Query().Get("query"), r.URL.Query().Get("type"), year)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (a *App) tmdbPoster(w http.ResponseWriter, r *http.Request) {
	data, contentType, err := a.tmdb.Image(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *App) tmdbDetails(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid TMDB id"))
		return
	}
	details, err := a.tmdb.Details(r.Context(), r.PathValue("type"), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (a *App) tmdbRename(w http.ResponseWriter, r *http.Request) {
	instance, ok := a.storage(r.PathValue("id"), w)
	if !ok {
		return
	}
	var body struct {
		Path      string `json:"path"`
		MediaType string `json:"media_type"`
		TMDBID    int    `json:"tmdb_id"`
		Template  string `json:"template"`
		Preview   bool   `json:"preview"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	details, err := a.tmdb.Details(r.Context(), body.MediaType, body.TMDBID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	entries, err := instance.List(r.Context(), body.Path)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	changes := make([]renamer.Change, 0)
	year := ""
	if len(details.ReleaseDate) >= 4 {
		year = details.ReleaseDate[:4]
	}
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		values := map[string]string{
			"title": details.Title, "title_original": details.OriginalTitle, "year": year, "tmdbid": strconv.Itoa(details.ID), "ext": path.Ext(entry.Name),
		}
		if episode, found := renamer.EpisodeMetadata(entry.Name); found {
			for _, key := range []string{"season", "episode", "episode_name", "quality"} {
				if value := episode[key]; value != "" {
					values[key] = value
				}
			}
			if body.MediaType == "tv" {
				season, _ := strconv.Atoi(episode["season"])
				episodeNumber, _ := strconv.Atoi(episode["episode"])
				episodeDetails, detailErr := a.tmdb.Episode(r.Context(), body.TMDBID, season, episodeNumber)
				if detailErr != nil {
					writeError(w, http.StatusBadGateway, detailErr)
					return
				}
				values["episode_name"] = episodeDetails.Name
			}
		}
		name, err := tmdb.Render(body.Template, values)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if name != "" && name != entry.Name {
			changes = append(changes, renamer.Change{From: entry.Name, To: name})
		}
	}
	changes, err = renamer.Validate(changes, entries)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !body.Preview {
		if err := renamer.Execute(r.Context(), instance, storage.CleanRemote(body.Path), changes); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": body.Preview, "details": details, "changes": changes})
}

func (a *App) diskConfig() (config.Config, error) {
	if a.config.Path == "" {
		return config.Config{}, fmt.Errorf("configuration persistence is unavailable")
	}
	return config.LoadDisk(a.config.Path)
}

func (a *App) getConfig(w http.ResponseWriter, _ *http.Request) {
	cfg, err := a.diskConfig()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg.Redacted(), "restart_required": false, "backup_available": fileExists(a.config.Path + ".bak")})
}

func (a *App) putConfig(w http.ResponseWriter, r *http.Request) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	current, err := a.diskConfig()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var candidate config.Config
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, fmt.Errorf("request body must contain one JSON object"))
		return
	}
	config.PreserveSecrets(&candidate, current)
	candidate.Version, candidate.Path = config.CurrentVersion, a.config.Path
	if err := config.Save(a.config.Path, candidate); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "restart_required": true, "config": candidate.Redacted()})
}

func (a *App) restoreConfig(w http.ResponseWriter, _ *http.Request) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if a.config.Path == "" {
		writeError(w, http.StatusConflict, fmt.Errorf("configuration persistence is unavailable"))
		return
	}
	cfg, err := config.Restore(a.config.Path)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "restored", "restart_required": true, "config": cfg.Redacted()})
}

func (a *App) resetWebhookToken(w http.ResponseWriter, _ *http.Request) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	cfg, err := a.diskConfig()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	cfg.WebhookToken = token
	if err := config.Save(a.config.Path, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "webhook_token": token, "restart_required": true})
}

func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }

func (a *App) listHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, http.StatusOK, a.history.Snapshot(r.URL.Query().Get("task_id"), limit))
}

func (a *App) streamHistory(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming is unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	writeSSE := func(event history.Event) bool {
		data, err := json.Marshal(event)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: history\ndata: %s\n\n", event.ID, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	for _, event := range reverseHistory(a.history.Snapshot(r.URL.Query().Get("task_id"), 50)) {
		if !writeSSE(event) {
			return
		}
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	events := a.history.Subscribe(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, open := <-events:
			if !open {
				return
			}
			if taskID := r.URL.Query().Get("task_id"); taskID != "" && event.TaskID != taskID {
				continue
			}
			if !writeSSE(event) {
				return
			}
		}
	}
}

func reverseHistory(events []history.Event) []history.Event {
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	return events
}

func (a *App) storage(id string, w http.ResponseWriter) (storage.Storage, bool) {
	instance, exists := a.storages[id]
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown storage %q", id))
		return nil, false
	}
	return instance, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (a *App) webhookRun(w http.ResponseWriter, r *http.Request) {
	if !a.validToken(r) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}
	var body struct {
		TaskID string `json:"task_id"`
		Path   string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.manager.Enqueue(body.TaskID, body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (a *App) publicWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.validToken(r) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}
	var payload struct {
		Event    string `json:"event"`
		StrmTask string `json:"strmtask"`
		SavePath string `json:"savepath"`
		PathFix  string `json:"xlist_path_fix"`
		Refresh  bool   `json:"refresh"`
		Delay    int    `json:"delay"`
		Task     struct {
			Name           string                    `json:"name"`
			StoragePath    string                    `json:"storage_path"`
			DirTimeCheck   *bool                     `json:"dir_time_check"`
			Incremental    *bool                     `json:"incremental"`
			KeepLocalAsset *bool                     `json:"keep_local_asset"`
			Plugins        map[string]map[string]any `json:"plugins"`
		} `json:"task"`
		STRM struct {
			MediaExt  []string `json:"media_ext"`
			CopyExt   []string `json:"copy_ext"`
			MediaSize int64    `json:"media_size"`
		} `json:"strm"`
		Data struct {
			Driver   string `json:"driver"`
			SavePath string `json:"savepath"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if payload.Delay < 0 || payload.Delay > 86400 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("delay must be 0-86400 seconds"))
		return
	}
	if payload.Event == "web_save" {
		a.webSave(w, r, payload.Data.Driver, payload.Data.SavePath, time.Duration(payload.Delay)*time.Second)
		return
	}
	taskNames := []string{payload.Task.Name}
	if payload.Event == "qas_strm" || payload.Event == "cs_strm" || (payload.Event == "" && payload.StrmTask != "") {
		taskNames = strings.Split(payload.StrmTask, ",")
		if payload.Task.StoragePath == "" {
			payload.Task.StoragePath = payload.SavePath
		}
	} else if payload.Event != "a_task" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported webhook event %q", payload.Event))
		return
	}
	plugins, err := webhookPlugins(payload.Task.Plugins)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	type refreshRequest struct {
		storage storage.Refresher
		path    string
	}
	requests := make([]task.BatchRequest, 0, len(taskNames))
	refreshes := make([]refreshRequest, 0, len(taskNames))
	queued := make([]string, 0, len(taskNames))
	for _, name := range taskNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, cfg, err := a.manager.ResolveTask(name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		storagePath := payload.Task.StoragePath
		if payload.PathFix != "" {
			storagePath = applyPathFix(storagePath, payload.PathFix)
		}
		if payload.Task.Incremental != nil {
			cfg.Incremental = *payload.Task.Incremental
		}
		if payload.Task.DirTimeCheck != nil {
			cfg.DirTimeCheck = *payload.Task.DirTimeCheck
		}
		if payload.Task.KeepLocalAsset != nil {
			if *payload.Task.KeepLocalAsset {
				cfg.KeepLocal = appendUnique(cfg.KeepLocal, "*.nfo", "*.jpg", "*.png")
			} else {
				cfg.KeepLocal = nil
			}
		}
		if payload.Refresh {
			instance := a.storages[cfg.StorageID]
			refresher, supported := instance.(storage.Refresher)
			if !supported {
				writeError(w, http.StatusBadRequest, fmt.Errorf("storage %q does not support refresh", cfg.StorageID))
				return
			}
			refreshPath := storagePath
			if refreshPath == "" {
				refreshPath = cfg.Source
			}
			refreshes = append(refreshes, refreshRequest{storage: refresher, path: refreshPath})
		}
		if len(payload.STRM.MediaExt) > 0 {
			cfg.MediaExt = normalizeExtensions(payload.STRM.MediaExt)
		}
		if len(payload.STRM.CopyExt) > 0 {
			cfg.CopyExt = normalizeExtensions(payload.STRM.CopyExt)
		}
		if payload.STRM.MediaSize > 0 {
			cfg.MinSize = payload.STRM.MediaSize * 1024 * 1024
		}
		if len(plugins) > 0 {
			cfg.Plugins = append([]config.PluginConfig(nil), plugins...)
		}
		requests = append(requests, task.BatchRequest{TaskID: id, Path: storagePath, Config: &cfg})
		queued = append(queued, id)
	}
	if len(queued) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no tasks requested"))
		return
	}
	for _, refresh := range refreshes {
		if err := refresh.storage.Refresh(r.Context(), refresh.path); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	if err := a.manager.EnqueueBatch(requests, time.Duration(payload.Delay)*time.Second); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "tasks": queued, "delay": payload.Delay})
}

func (a *App) webSave(w http.ResponseWriter, r *http.Request, driver, savePath string, delay time.Duration) {
	driverTypes := map[string]string{"cloud189": "189", "quark": "quark", "open115": "115"}
	storageType, supported := driverTypes[strings.ToLower(strings.TrimSpace(driver))]
	if !supported {
		writeWebSaveError(w, http.StatusBadRequest, fmt.Errorf("unsupported web save driver %q", driver))
		return
	}
	if strings.TrimSpace(savePath) == "" || len(savePath) > 4096 || strings.ContainsRune(savePath, '\x00') {
		writeWebSaveError(w, http.StatusBadRequest, fmt.Errorf("web save path is invalid"))
		return
	}
	cleanedPath := storage.CleanRemote(savePath)
	type candidate struct {
		id     string
		config config.TaskConfig
	}
	candidates := make([]candidate, 0)
	bestLength := -1
	for _, storageConfig := range a.config.Storages {
		if !strings.EqualFold(storageConfig.Type, storageType) {
			continue
		}
		for _, taskID := range a.manager.MatchTasks(storageConfig.ID, cleanedPath) {
			_, taskConfig, err := a.manager.ResolveTask(taskID)
			if err != nil {
				writeWebSaveError(w, http.StatusConflict, err)
				return
			}
			sourceLength := len(storage.CleanRemote(taskConfig.Source))
			if sourceLength > bestLength {
				bestLength = sourceLength
				candidates = candidates[:0]
			}
			if sourceLength == bestLength {
				candidates = append(candidates, candidate{id: taskID, config: taskConfig})
			}
		}
	}
	if len(candidates) == 0 {
		writeWebSaveError(w, http.StatusNotFound, fmt.Errorf("no task matches %s path %q", driver, cleanedPath))
		return
	}
	if len(candidates) != 1 {
		writeWebSaveError(w, http.StatusConflict, fmt.Errorf("web save path %q matches multiple equally specific tasks", cleanedPath))
		return
	}
	matched := candidates[0]
	if refresher, ok := a.storages[matched.config.StorageID].(storage.Refresher); ok {
		if err := refresher.Refresh(r.Context(), cleanedPath); err != nil {
			writeWebSaveError(w, http.StatusBadGateway, fmt.Errorf("refresh web save path: %w", err))
			return
		}
	}
	if err := a.manager.EnqueueBatch([]task.BatchRequest{{TaskID: matched.id, Path: cleanedPath}}, delay); err != nil {
		writeWebSaveError(w, http.StatusConflict, err)
		return
	}
	name := matched.config.Name
	if name == "" {
		name = matched.id
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"message": "matching task queued",
		"task":    map[string]string{"name": name, "storage_path": cleanedPath},
	})
}

func writeWebSaveError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"success": false, "message": err.Error()})
}

func (a *App) cloudDrive2Webhook(w http.ResponseWriter, r *http.Request) {
	if !a.validToken(r) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}
	if len(a.config.Integrations.CloudDrive2) == 0 {
		writeError(w, http.StatusConflict, fmt.Errorf("CloudDrive2 integration is not configured"))
		return
	}
	var payload struct {
		EventCategory string `json:"event_category"`
		EventName     string `json:"event_name"`
		Data          []struct {
			Action          string `json:"action"`
			IsDir           any    `json:"is_dir"`
			SourceFile      string `json:"source_file"`
			DestinationFile string `json:"destination_file"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	queued := make(map[string]string)
	for _, event := range payload.Data {
		changedPath := event.DestinationFile
		if changedPath == "" {
			changedPath = event.SourceFile
		}
		storageID, remotePath, ok := mapCloudDrivePath(changedPath, a.config.Integrations.CloudDrive2)
		if !ok {
			continue
		}
		if !valueIsTrue(event.IsDir) {
			remotePath = path.Dir(remotePath)
		}
		for _, taskID := range a.manager.MatchTasks(storageID, remotePath) {
			if previous, exists := queued[taskID]; !exists || len(remotePath) < len(previous) {
				queued[taskID] = remotePath
			}
		}
	}
	ids := make([]string, 0, len(queued))
	for taskID, remotePath := range queued {
		if err := a.manager.Enqueue(taskID, remotePath); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		ids = append(ids, taskID)
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored", "reason": "no matching task"})
		return
	}
	sort.Strings(ids)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "tasks": ids})
}

func (a *App) moviePilotWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.validToken(r) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}
	if len(a.config.Integrations.MoviePilot) == 0 {
		writeError(w, http.StatusConflict, fmt.Errorf("MoviePilot integration is not configured"))
		return
	}
	var payload struct {
		Type string `json:"type"`
		Data struct {
			TransferInfo struct {
				TargetDirItem struct {
					Path string `json:"path"`
				} `json:"target_diritem"`
				FileListNew []string `json:"file_list_new"`
			} `json:"transferinfo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if payload.Type != "transfer.complete" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored", "reason": "event is not transfer.complete"})
		return
	}
	paths := append([]string(nil), payload.Data.TransferInfo.FileListNew...)
	if payload.Data.TransferInfo.TargetDirItem.Path != "" {
		paths = append(paths, payload.Data.TransferInfo.TargetDirItem.Path)
	}
	queued := make(map[string]string)
	for _, localPath := range paths {
		storageID, remotePath, ok := mapExternalPath(localPath, a.config.Integrations.MoviePilot)
		if !ok {
			continue
		}
		if filepath.Ext(remotePath) != "" {
			remotePath = path.Dir(remotePath)
		}
		for _, taskID := range a.manager.MatchTasks(storageID, remotePath) {
			if previous, exists := queued[taskID]; !exists || len(remotePath) < len(previous) {
				queued[taskID] = remotePath
			}
		}
	}
	ids := make([]string, 0, len(queued))
	for taskID, remotePath := range queued {
		if err := a.manager.Enqueue(taskID, remotePath); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		ids = append(ids, taskID)
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored", "reason": "no matching task"})
		return
	}
	sort.Strings(ids)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "tasks": ids})
}

func mapExternalPath(input string, mappings map[string]string) (string, string, bool) {
	cleaned := filepath.ToSlash(filepath.Clean(input))
	bestSource, bestTarget := "", ""
	for source, target := range mappings {
		source = strings.TrimRight(filepath.ToSlash(filepath.Clean(source)), "/")
		if cleaned == source || strings.HasPrefix(cleaned, source+"/") {
			if len(source) > len(bestSource) {
				bestSource, bestTarget = source, target
			}
		}
	}
	if bestSource == "" {
		return "", "", false
	}
	storageID, targetRoot := bestTarget, "/"
	if slash := strings.Index(bestTarget, "/"); slash >= 0 {
		storageID, targetRoot = bestTarget[:slash], "/"+bestTarget[slash+1:]
	}
	if storageID == "" {
		return "", "", false
	}
	return storageID, storage.CleanRemote(path.Join(targetRoot, strings.TrimPrefix(cleaned, bestSource))), true
}

func mapCloudDrivePath(input string, mappings map[string]string) (string, string, bool) {
	cleaned := storage.CleanRemote(input)
	bestSource, bestTarget := "", ""
	for source, target := range mappings {
		source = storage.CleanRemote(source)
		if cleaned == source || strings.HasPrefix(cleaned, strings.TrimRight(source, "/")+"/") {
			if len(source) > len(bestSource) {
				bestSource, bestTarget = source, target
			}
		}
	}
	if bestSource == "" {
		return "", "", false
	}
	storageID, targetRoot := bestTarget, "/"
	if slash := strings.Index(bestTarget, "/"); slash >= 0 {
		storageID, targetRoot = bestTarget[:slash], "/"+bestTarget[slash+1:]
	}
	if storageID == "" {
		return "", "", false
	}
	remainder := strings.TrimPrefix(cleaned, bestSource)
	return storageID, storage.CleanRemote(path.Join(targetRoot, remainder)), true
}

func valueIsTrue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	case float64:
		return typed != 0
	default:
		return false
	}
}

func applyPathFix(sourcePath, mapping string) string {
	left, right, ok := strings.Cut(mapping, ":")
	if !ok {
		return sourcePath
	}
	left, right = strings.TrimRight(storage.CleanRemote(left), "/"), storage.CleanRemote(right)
	cleaned := storage.CleanRemote(sourcePath)
	if cleaned == right {
		return left
	}
	if strings.HasPrefix(cleaned, strings.TrimRight(right, "/")+"/") {
		return left + strings.TrimPrefix(cleaned, strings.TrimRight(right, "/"))
	}
	return cleaned
}

func webhookPlugins(values map[string]map[string]any) ([]config.PluginConfig, error) {
	result := make([]config.PluginConfig, 0, len(values))
	types := make([]string, 0, len(values))
	for pluginType := range values {
		types = append(types, pluginType)
	}
	sort.Strings(types)
	for _, pluginType := range types {
		settings := values[pluginType]
		if pluginType != "skip_regex" && pluginType != "replace_regex" && pluginType != "filename" && pluginType != "filename_skip" {
			return nil, fmt.Errorf("unsupported webhook plugin %q", pluginType)
		}
		pattern, _ := settings["pattern"].(string)
		replacement, _ := settings["replacement"].(string)
		if pattern == "" {
			return nil, fmt.Errorf("webhook plugin %q requires pattern", pluginType)
		}
		plugin := config.PluginConfig{Type: pluginType, Pattern: pattern, Replacement: replacement}
		if pluginType == "filename_skip" {
			var err error
			if plugin.MatchMode, err = webhookStringSetting(settings, "match_mode"); err != nil {
				return nil, err
			}
			if plugin.FilterMode, err = webhookStringSetting(settings, "filter_mode"); err != nil {
				return nil, err
			}
			if plugin.DirectoryOnly, err = webhookBoolSetting(settings, "directory_only"); err != nil {
				return nil, err
			}
			if plugin.CaseSensitive, err = webhookBoolSetting(settings, "case_sensitive"); err != nil {
				return nil, err
			}
			if err := config.ValidatePlugin(plugin); err != nil {
				return nil, fmt.Errorf("webhook %w", err)
			}
		}
		result = append(result, plugin)
	}
	return result, nil
}

func webhookStringSetting(settings map[string]any, name string) (string, error) {
	value, exists := settings[name]
	if !exists {
		return "", nil
	}
	result, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("webhook filename_skip %s must be a string", name)
	}
	return result, nil
}

func webhookBoolSetting(settings map[string]any, name string) (bool, error) {
	value, exists := settings[name]
	if !exists {
		return false, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("webhook filename_skip %s must be a boolean", name)
	}
	return result, nil
}

func webhookCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		next.ServeHTTP(w, r)
	})
}

func webhookOptions(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func normalizeExtensions(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		if value != "" {
			result = append(result, strings.ToLower(value))
		}
	}
	return result
}

func appendUnique(values []string, additions ...string) []string {
	result := append([]string(nil), values...)
	seen := make(map[string]struct{}, len(result)+len(additions))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (a *App) embyDelete(w http.ResponseWriter, r *http.Request) {
	if !a.validToken(r) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}
	var event struct {
		Event string `json:"Event"`
		Item  struct {
			Path string `json:"Path"`
		} `json:"Item"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if event.Event != "item.deleted" && event.Event != "ItemDeleted" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if err := a.deleteFromSTRM(r.Context(), event.Item.Path); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (a *App) deleteFromSTRM(ctx context.Context, strmPath string) error {
	abs, err := filepath.Abs(strmPath)
	if err != nil {
		return err
	}
	allowed := false
	for _, cfg := range a.config.Tasks {
		relative, relErr := filepath.Rel(cfg.Destination, abs)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("path is outside configured destinations")
	}
	if !strings.EqualFold(filepath.Ext(abs), ".strm") {
		return fmt.Errorf("path is not a STRM file")
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("STRM path must be a regular file")
	}
	if info.Size() > 64<<10 {
		return fmt.Errorf("STRM file is unexpectedly large")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	streamURL, err := url.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	parts := strings.Split(strings.Trim(streamURL.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "stream" {
		return fmt.Errorf("STRM is not managed by this server")
	}
	storageID, err := url.PathUnescape(parts[1])
	if err != nil {
		return err
	}
	instance, exists := a.storages[storageID]
	if !exists {
		return fmt.Errorf("unknown storage")
	}
	signedValue, err := signedStreamLocator(streamURL.Query())
	if err != nil {
		return err
	}
	if !signature.Valid(a.config.WebhookToken, storageID, signedValue, streamURL.Query().Get("sig")) {
		return fmt.Errorf("STRM signature is invalid")
	}
	remotePath, err := resolveStreamLocator(ctx, instance, streamURL.Query())
	if err != nil {
		return err
	}
	if err := instance.Delete(ctx, remotePath); err != nil {
		return err
	}
	return os.Remove(abs)
}

func (a *App) stream(w http.ResponseWriter, r *http.Request) {
	storageID := r.PathValue("storage")
	instance, exists := a.storages[storageID]
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown storage"))
		return
	}
	signedValue, err := signedStreamLocator(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !signature.Valid(a.config.WebhookToken, storageID, signedValue, r.URL.Query().Get("sig")) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid stream signature"))
		return
	}
	remotePath, err := resolveStreamLocator(r.Context(), instance, r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if streamer, ok := instance.(storage.HTTPStreamer); ok {
		if err := streamer.Stream(w, r, remotePath); err != nil {
			writeError(w, http.StatusBadGateway, err)
		}
		return
	}
	directURL, err := instance.DirectURL(r.Context(), remotePath)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	parsed, err := urlpolicy.ParseHTTP(directURL, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("storage returned an invalid direct URL: %w", err))
		return
	}
	http.Redirect(w, r, parsed.String(), http.StatusFound)
}

func signedStreamLocator(values url.Values) (string, error) {
	remotePath := values.Get("path")
	fileID := values.Get("id")
	if (remotePath == "") == (fileID == "") {
		return "", fmt.Errorf("exactly one of path or id is required")
	}
	if fileID == "" {
		return storage.CleanRemoteExact(remotePath), nil
	}
	if len(fileID) > 1024 {
		return "", fmt.Errorf("file ID is too long")
	}
	return "id:" + fileID, nil
}

func resolveStreamLocator(ctx context.Context, instance storage.Storage, values url.Values) (string, error) {
	if values.Get("id") == "" {
		remotePath := storage.CleanRemoteExact(values.Get("path"))
		return remotePath, nil
	}
	fileIDSource, ok := instance.(storage.FileIDStorage)
	if !ok {
		err := fmt.Errorf("storage does not support stable file IDs")
		return "", err
	}
	resolved, err := fileIDSource.ResolveFileID(ctx, values.Get("id"))
	if err != nil {
		wrapped := fmt.Errorf("resolve stable file ID: %w", err)
		return "", wrapped
	}
	remotePath := storage.CleanRemoteExact(resolved)
	return remotePath, nil
}

func (a *App) validToken(r *http.Request) bool {
	provided := r.URL.Query().Get("token")
	if provided == "" {
		provided = r.PathValue("token")
	}
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.config.WebhookToken)) == 1
}

func (a *App) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.config.Admin.Username == "" {
			next.ServeHTTP(w, r)
			return
		}
		username, password, ok := r.BasicAuth()
		validUser := subtle.ConstantTimeCompare([]byte(username), []byte(a.config.Admin.Username)) == 1
		validPass := subtle.ConstantTimeCompare([]byte(password), []byte(a.config.Admin.Password)) == 1
		if !ok || !validUser || !validPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="SmartStrm"`)
			writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; object-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

//go:embed ui/index.html
var indexHTML string
