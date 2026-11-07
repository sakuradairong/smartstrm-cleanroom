# GitHub Community Capability Review: STRM Generation, Media Automation, and Cloud Drive Reconciliation

Date: 2026-11-05  
Status: Completed Research Report  
Target System: SmartStrm Clean-room (Go Monolith)  
Output Path: `docs/research/github-community-capability-review-2026-11-05.md`

---

## 1. Executive Summary and Scope

This research evaluates mature open-source tools on GitHub that share functionality with SmartStrm. SmartStrm generates and maintains `.strm` files, schedules automation jobs, receives webhooks, and proxies media streams from local and cloud storage. The clean-room Go implementation handles core tasks well, but earlier reviews identified architectural risks around state reconciliation, output ownership, and deletion safety. The current working tree already rejects overlapping task outputs and limits toolbox cleanup to signed STRM URLs matching the selected task's storage and source path. The remaining immediate gap before this review was operator visibility: cleanup had no read-only plan showing scanned, owned, and retained counts before confirmation.

We investigated seven projects representing different aspects of media automation and cloud storage reconciliation:
1. **rclone** (`rclone/rclone`): The benchmark for cloud storage sync, dry runs, and conservative deletion.
2. **AList** (`alist-org/alist`): High-concurrency multi-storage gateway, WebDAV provider, and 302 stream router.
3. **MoviePilot** (`jxxghp/MoviePilot`): Full-lifecycle NAS media manager with transfer history tracking, TMDB recognition, and media server library refresh.
4. **Auto_Symlink** (`shenxianmq/Auto_Symlink`): Dedicated cloud-to-local symlink and STRM generator with filesystem event monitoring and broken link cleanup.
5. **openlist-strm** (`konghanghang/openlist-strm`): Dedicated Go and SQLite OpenList STRM generator with per-task rate limiting and dry-run preview.
6. **embyExternalUrl** (`bpking1/embyExternalUrl`): High-performance Emby and Jellyfin reverse proxy for dynamic `PlaybackInfo` rewriting and 302 cloud redirection.
7. **restic** (`restic/restic`): Cryptographic snapshot and tree manifest architecture, repository locking, and two-phase mark-and-sweep garbage collection.

Every non-obvious technical claim in this document links directly to an official GitHub repository, source file, documentation page, or release note. We explicitly distinguish verified source facts from design inferences. Community evidence consistently favors a discovery or dry-run phase before destructive execution. This review therefore recommends the cleanup preview delivered in the current increment, followed by a task-scoped manifest and deletion cap as the larger architectural step.

---

## 2. Project Analysis and Primary Sources

### 2.1 rclone (`rclone/rclone`)

- **Primary Repository**: [https://github.com/rclone/rclone](https://github.com/rclone/rclone)
- **Latest Evaluated Release**: [v1.75.2](https://github.com/rclone/rclone/releases/tag/v1.75.2)
- **Primary Source Code References**:
  - Sync and deletion engine: [`fs/sync/sync.go`](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go)
  - Rename tracking implementation: [`fs/sync/trackrenames/trackrenames.go`](https://github.com/rclone/rclone/blob/master/fs/sync/trackrenames/trackrenames.go)
  - Operations and backup flags: [`fs/operations/operationsflags/operationsflags.go`](https://github.com/rclone/rclone/blob/master/fs/operations/operationsflags/operationsflags.go)
  - Path filtering and globbing: [`fs/filter/filter.go`](https://github.com/rclone/rclone/blob/master/fs/filter/filter.go)
  - Fast directory listing: [`fs/operations/lsjson.go`](https://github.com/rclone/rclone/blob/master/fs/operations/lsjson.go)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: Rclone doesn't use a central relational database. Instead, it computes real-time diffs between source and destination remotes using timestamps, file sizes, and hashes ([`fs/sync/sync.go`](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go#L111)). For reproducible synchronization, it accepts manifest input through `--files-from` and supports external comparison trees via `--compare-dest`.

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: The `--dry-run` flag runs the entire directory walk, filter matching, and action planning logic without modifying destination files ([`fs/operations/operations.go`](https://github.com/rclone/rclone/blob/master/fs/operations/operations.go)). It logs planned additions, copies, deletions, and moves with `NOTICE` level tags, giving operators complete visibility into changes before committing them to disk.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: Rclone provides industry-leading safety controls during destructive synchronization:
   - `--delete-after`: Defers all file deletions until after every transfer finishes successfully. If any transfer fails, rclone aborts the deletion phase entirely ([`fs/sync/sync.go#L849`](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go#L849)).
   - `--backup-dir`: Instead of unlinking destination files, rclone moves deleted or overwritten files into a designated, timestamped backup directory ([`fs/sync/sync.go#L245`](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go#L245)).
   - `--max-delete <N>`: Caps deletions during execution. In [v1.75.2's `StatsInfo.DeleteFile`](https://github.com/rclone/rclone/blob/v1.75.2/fs/accounting/stats.go), the next deletion returns a fatal error when it would exceed N; up to N files may already have been deleted, and those deletions are not rolled back. Rejecting an oversized deletion plan before any mutation requires a separate preflight phase, as proposed for SmartStrm below.
   - `operations.OverlappingFilterCheck`: Rejects operations where source and destination paths overlap, stopping recursive data destruction before it starts ([`fs/sync/sync.go#L112`](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go#L112)).

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: The `--track-renames` feature matches missing destination files against new source files using file size and hash checksums ([`fs/sync/trackrenames/trackrenames.go`](https://github.com/rclone/rclone/blob/master/fs/sync/trackrenames/trackrenames.go)). When a match occurs, rclone issues a remote rename or server-side move instead of deleting the destination file and re-uploading the source content.

5. **Task Generation and Versioning**:  
   *Verified Fact*: In addition to CLI operation, rclone provides an RPC daemon (`rclone rcd`) with job IDs, asynchronous task tracking, and cancellation controls via `core/command` and `job/status` endpoints ([`fs/rc/jobs/jobs.go`](https://github.com/rclone/rclone/blob/master/fs/rc/jobs/jobs.go)).

6. **Large-Directory Handling**:  
   *Verified Fact*: The `--fast-list` flag loads directories in large paginated chunks where supported by backend APIs (such as S3 or Google Drive), drastically cutting down HTTP roundtrips. Rclone streams directory entries directly into worker pipelines instead of holding complete directory trees in memory ([`fs/operations/lsjson.go`](https://github.com/rclone/rclone/blob/master/fs/operations/lsjson.go)).

7. **Media-Server Refresh**:  
   *Verified Fact*: Rclone focuses on storage synchronization. It doesn't include native Emby, Jellyfin, or Plex API clients, leaving media server notifications to downstream scripts or wrapper tools.

8. **Safety Controls**:  
   *Verified Fact*: Beyond `--backup-dir` and `--max-delete`, rclone enforces `--read-only` mounts, transaction rate limits (`--tpslimit`), bandwidth limits (`--bwlimit`), and isolated filename suffixes (`--suffix`).

---

### 2.2 AList (`alist-org/alist`)

- **Primary Repository**: [https://github.com/alist-org/alist](https://github.com/alist-org/alist)
- **Latest Evaluated Release**: [v3.66.0](https://github.com/alist-org/alist/releases/tag/v3.66.0)
- **Primary Source Code References**:
  - Task manager and workers: [`pkg/task/manager.go`](https://github.com/alist-org/alist/blob/main/pkg/task/manager.go)
  - 115 Open Platform storage driver: [`drivers/115_open/driver.go`](https://github.com/alist-org/alist/blob/main/drivers/115_open/driver.go)
  - 123 Pan storage driver: [`drivers/123/driver.go`](https://github.com/alist-org/alist/blob/main/drivers/123/driver.go)
  - Driver base interface: [`drivers/base/driver.go`](https://github.com/alist-org/alist/blob/main/drivers/base/driver.go)
  - Task data model: [`internal/model/task.go`](https://github.com/alist-org/alist/blob/main/internal/model/task.go)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: AList provides a virtual filesystem over heterogeneous cloud storages. It maintains a database (SQLite, MySQL, or PostgreSQL) storing storage mount configurations, user permissions, and background task records ([`internal/model/task.go`](https://github.com/alist-org/alist/blob/main/internal/model/task.go)). It doesn't write local `.strm` files directly, serving virtual endpoints instead.

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: AList doesn't provide a standalone offline dry-run mode for file operations. The web interface displays real-time task queue status, source and destination paths, and progress percentages for background copy and upload jobs.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: Cloud file deletion routes through each driver's `Remove` implementation. Drivers check if the remote provider supports a trash or recycle bin (for example 115 and Aliyun drive). When supported, items move to trash rather than undergoing immediate hard deletion.

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: The driver base interface mandates `Rename` and `Move` methods ([`drivers/base/driver.go`](https://github.com/alist-org/alist/blob/main/drivers/base/driver.go)). Drivers implement native server-side directory moves. When cloud APIs fail mid-operation, partial moves remain on the remote storage without automatic rollback.

5. **Task Generation and Versioning**:  
   *Verified Fact*: AList implements an asynchronous task queue in [`pkg/task/manager.go`](https://github.com/alist-org/alist/blob/main/pkg/task/manager.go). The manager supports configurable worker pools, task cancellation via Go context, retry policies, and persistent status tracking across restarts.

6. **Large-Directory Handling**:  
   *Verified Fact*: Storage drivers implement paginated directory listings using cursor tokens or page numbers. The frontend uses virtualized scrolling and lazy directory loading to handle directories with tens of thousands of items without freezing the browser DOM.

7. **Media-Server Refresh**:  
   *Verified Fact*: AList exposes standard WebDAV and HTTP redirect endpoints. It relies on external reverse proxies or community scripts (such as `emby2Alist`) to intercept media server playback requests.

8. **Safety Controls**:  
   *Verified Fact*: AList protects operations with role-based access control, signed download tokens with TTL expiration, path-level permission masks, and API rate limiters.

---

### 2.3 MoviePilot (`jxxghp/MoviePilot`)

- **Primary Repository**: [https://github.com/jxxghp/MoviePilot](https://github.com/jxxghp/MoviePilot)
- **Latest Evaluated Release**: [v3.2.4](https://github.com/jxxghp/MoviePilot/releases/tag/v3.2.4)
- **Primary Source Code References**:
  - Transfer history model: [`app/db/models/transferhistory.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/db/models/transferhistory.py)
  - Transfer execution chain: [`app/chain/transfer.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/chain/transfer.py)
  - Media server helper: [`app/helper/mediaserver.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/helper/mediaserver.py)
  - Media server event chain: [`app/chain/mediaserver.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/chain/mediaserver.py)
  - Transfer redo tools: [`app/agent/prompt/transfer_redo.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/agent/prompt/transfer_redo.py)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: MoviePilot maintains explicit output ownership using an embedded database table called `TransferHistory` ([`app/db/models/transferhistory.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/db/models/transferhistory.py)). Each record stores the source path (`src`), source storage, destination path (`dest`), destination storage, transfer mode (`move`, `copy`, `link`, or `softlink`), TMDB ID, media category, file size, and creation timestamp. This ledger enables deterministic tracking of every generated file.

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: When users configure directory scraping or manual organization, MoviePilot previews target directory structures, extracted titles, season and episode numbers, and resulting file names in the web interface before executing filesystem mutations.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: MoviePilot's cleanup relies on its transfer database. Because the system knows exactly which files it created, it can remove broken links or obsolete outputs without walking destination folders blindly. It also supports moving obsolete items to the trash folder rather than permanently deleting them.

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: MoviePilot includes transfer redo and history reversal tools ([`app/agent/prompt/transfer_redo.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/agent/prompt/transfer_redo.py)). If a media item was recognized incorrectly, the operator can trigger a rollback or redo action that reads the historical record, unlinks the generated target, and moves the source file back.

5. **Task Generation and Versioning**:  
   *Verified Fact*: The system coordinates background workflows using APScheduler and event-driven chains ([`app/chain/transfer.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/chain/transfer.py)). Tasks run concurrently or sequentially based on worker limits, with execution records preserved in the SQLite database.

6. **Large-Directory Handling**:  
   *Verified Fact*: Directory scans yield file generators to avoid loading whole folder trees into memory. The web UI uses paginated API endpoints to browse transfer history and file collections.

7. **Media-Server Refresh**:  
   *Verified Fact*: MoviePilot features first-class Emby, Jellyfin, and Plex integrations ([`app/helper/mediaserver.py`](https://github.com/jxxghp/MoviePilot/blob/v2/app/helper/mediaserver.py)). Upon completing a file transfer, it immediately calls the media server API to refresh only the affected library path (`refresh_library_by_path`), avoiding slow full-library scans.

8. **Safety Controls**:  
   *Verified Fact*: MoviePilot enforces minimum file size thresholds (skipping trailers and sample clips), exclusion regex rules, and directory collision checks to ensure target paths never overwrite unrelated files.

---

### 2.4 Auto_Symlink (`shenxianmq/Auto_Symlink`)

- **Primary Repository**: [https://github.com/shenxianmq/Auto_Symlink](https://github.com/shenxianmq/Auto_Symlink)
- **Latest Evaluated Release / Tag**: [0.1.1](https://github.com/shenxianmq/Auto_Symlink/tree/0.1.1) (Source references verified via official code repository and public archival fork [`berry8838/Auto_Symlink_copy`](https://github.com/berry8838/Auto_Symlink_copy))
- **Primary Source Code References**:
  - Symlink and STRM cleaner: [`autosync/SymlinkChecker.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkChecker.py)
  - Directory cleaner: [`autosync/SymlinkDirChecker.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkDirChecker.py)
  - Filesystem watcher: [`watcher/FileWatcher.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/watcher/FileWatcher.py)
  - Metadata copy engine: [`autosync/MetadataCopyer.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/MetadataCopyer.py)
  - Scheduled task runner: [`task_run.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/task_run.py)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: Auto_Symlink doesn't maintain an external database ledger for generated symlinks or `.strm` files. It inspects destination directories directly on each run. For symlinks, it reads the link target via `os.readlink`. For `.strm` files, it parses the stored URL ([`autosync/SymlinkChecker.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkChecker.py#L32)).

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: Auto_Symlink lacks an offline dry-run mode. Changes apply directly to disk during scheduled passes or filesystem events.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: In [`autosync/SymlinkChecker.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkChecker.py), `check_and_remove_dead_symlink` removes dead symlinks when the source cloud mount path exists but the specific target file does not. For `.strm` files, it tests the URL with an HTTP request (`check_strm`) before unlinking.  
   *Design Inference*: The directory cleaner [`autosync/SymlinkDirChecker.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkDirChecker.py) uses `shutil.rmtree`. If an operator misconfigures source or target paths, or if the source cloud mount drops out unexpectedly, entire destination folders risk accidental deletion.

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: The file watcher [`watcher/FileWatcher.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/watcher/FileWatcher.py) listens for inotify `on_moved` events. When a source file is renamed, the watcher updates the destination symlink or `.strm` file directly, avoiding a full rescan of the parent directory.

5. **Task Generation and Versioning**:  
   *Verified Fact*: Runs periodic synchronization loops in background threads controlled by a simple sleep interval and cron schedule ([`task_run.py`](https://github.com/berry8838/Auto_Symlink_copy/blob/main/task_run.py)).

6. **Large-Directory Handling**:  
   *Verified Fact*: Uses standard Python `os.walk` recursion with a multi-threaded work queue. On large cloud-fuse mounts (such as CloudDrive2 or Rclone mounts), deep directory trees can encounter high latency and memory spikes during recursive traversal.

7. **Media-Server Refresh**:  
   *Verified Fact*: Auto_Symlink includes native Emby and Jellyfin API notifications. When a new symlink or `.strm` file is created, it sends an HTTP POST request to the media server library endpoint to trigger automatic rescanning.

8. **Safety Controls**:  
   *Verified Fact*: The system verifies that the cloud mount root (`cloud_path`) exists before performing deletions, which guards against unmounted drive wipes. It lacks a configurable maximum deletion threshold or backup staging directory.

---

### 2.5 openlist-strm (`konghanghang/openlist-strm`)

- **Primary Repository**: [https://github.com/konghanghang/openlist-strm](https://github.com/konghanghang/openlist-strm)
- **Latest Evaluated Release**: [v0.1.0](https://github.com/konghanghang/openlist-strm/releases/tag/v0.1.0)
- **Primary Source Code References**:
  - GORM and SQLite data models: [`backend/internal/storage/models.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/storage/models.go)
  - STRM generator implementation: [`backend/internal/strm/generator.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/strm/generator.go)
  - Cron scheduler and task management: [`backend/internal/scheduler/scheduler.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/scheduler/scheduler.go)
  - Alist API client: [`backend/internal/alist/client.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/alist/client.go)
  - Cron preview API handlers: [`backend/internal/api/handlers_cron_preview_test.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/api/handlers_cron_preview_test.go)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: This project uses Go and SQLite with GORM. In [`backend/internal/storage/models.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/storage/models.go), the `File` table records every processed file with `Path`, `Size`, `ModifiedAt`, `Hash`, and `STRMPath`. The `Task` table records task executions with `TaskID`, `Mode`, `FilesCreated`, `FilesDeleted`, and `FilesSkipped`. This provides a clean manifest for all generated `.strm` outputs.

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: Openlist-strm provides dedicated preview endpoints ([`backend/internal/api/handlers_cron_preview_test.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/api/handlers_cron_preview_test.go)). Operators can simulate scheduled tasks and inspect which files will be created or skipped before triggering real disk writes.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: Synchronization runs compare remote file lists against SQLite records. When a remote file is removed, the generator deletes the corresponding local `.strm` file and deletes the record from the database. Because ownership is tied to database IDs, multiple tasks can safely point to different subdirectories without mutual interference.

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: When a file changes on the remote storage, the system updates the SQLite path record and recreates the target `.strm` file. It doesn't yet implement transactional two-stage renames or rollback journals.

5. **Task Generation and Versioning**:  
   *Verified Fact*: Uses `robfig/cron/v3` in [`backend/internal/scheduler/scheduler.go`](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/scheduler/scheduler.go). Each task mapping (`Mapping` model) configures its own concurrency limit (`Concurrent`), sync mode (`incremental` or `full`), and STRM URL generation style (`alist_path` or direct `http_url`).

6. **Large-Directory Handling**:  
   *Verified Fact*: The Alist client requests directory listings with pagination and batch inserts records into SQLite to prevent memory bloat during full synchronization passes.

7. **Media-Server Refresh**:  
   *Verified Fact*: The system provides generic notification channels (such as webhooks and Telegram alerts in `backend/internal/notification/notification.go`), with direct Emby library refresh planned on its roadmap.

8. **Safety Controls**:  
   *Verified Fact*: Per-task concurrency settings prevent flooding cloud storage APIs and triggering rate-limit bans. It requires valid authentication tokens and sanitizes filesystem target paths.

---

### 2.6 embyExternalUrl (`bpking1/embyExternalUrl`)

- **Primary Repository**: [https://github.com/bpking1/embyExternalUrl](https://github.com/bpking1/embyExternalUrl)
- **Latest Evaluated Release**: [v0.4.8](https://github.com/bpking1/embyExternalUrl/releases/tag/v0.4.8)
- **Primary Source Code References**:
  - Main redirect router: [`emby2Alist/nginx/conf.d/emby.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/emby.js)
  - PlaybackInfo interceptor: [`emby2Alist/nginx/conf.d/modules/emby-playback-info.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/modules/emby-playback-info.js)
  - Common constants and flags: [`emby2Alist/nginx/conf.d/config/constant-common.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/config/constant-common.js)
  - STRM configuration: [`emby2Alist/nginx/conf.d/config/constant-strm.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/config/constant-strm.js)
  - Plex redirect router: [`plex2Alist/nginx/conf.d/plex.js`](https://github.com/bpking1/embyExternalUrl/blob/main/plex2Alist/nginx/conf.d/plex.js)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: This project acts as an Nginx and NJS reverse proxy rather than a static file generator. It doesn't write local files, so output manifests don't apply. It caches route mappings in Nginx shared memory dictionaries (`ngx.shared.routeL1Dict` and `routeL2Dict`).

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: Configurable log levels (`logLevel: 0/1/2/3` in [`emby2Alist/nginx/conf.d/config/constant-common.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/config/constant-common.js)) allow operators to test URL rewriting in log files before pointing client applications to the proxy.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: Because no static files are generated on disk, filesystem reconciliation is unnecessary. Stale routing entries expire automatically through shared memory TTL eviction.

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: Path resolution happens on demand during playback requests. If a remote file moves, the proxy resolves the new path on the next stream attempt.

5. **Task Generation and Versioning**:  
   *Verified Fact*: Runs as a stateless service with periodic background cache refresh tasks configured via [`emby2Alist/nginx/conf.d/common/periodics.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/common/periodics.js).

6. **Large-Directory Handling**:  
   *Verified Fact*: Bypasses directory scanning completely. By resolving individual items on demand when clients request playback, it operates with zero memory overhead regardless of total media library size.

7. **Media-Server Refresh**:  
   *Verified Fact*: Intercepts Emby, Jellyfin, and Plex requests dynamically at `/PlaybackInfo` and `/stream` endpoints ([`emby2Alist/nginx/conf.d/emby.js`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/emby.js#L14)). It rewrites internal media paths into direct 302 cloud redirects, delivering immediate playback acceleration without waiting for library scans.

8. **Safety Controls**:  
   *Verified Fact*: Features a reliable fallback switch (`fallbackUseOriginal` in [`emby2Alist/nginx/conf.d/emby.js#L54`](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/emby.js#L54)). If 302 resolution fails or an upstream cloud API returns an error, the proxy falls back to standard Emby streaming or transcoding, ensuring playback never breaks for the user.

---

### 2.7 restic (`restic/restic`)

- **Primary Repository**: [https://github.com/restic/restic](https://github.com/restic/restic)
- **Latest Evaluated Release**: [v0.19.1](https://github.com/restic/restic/releases/tag/v0.19.1)
- **Primary Source Code References**:
  - Prune and garbage collection: [`cmd/restic/cmd_prune.go`](https://github.com/restic/restic/blob/master/cmd/restic/cmd_prune.go)
  - Snapshot data structures: [`internal/data/snapshot.go`](https://github.com/restic/restic/blob/master/internal/data/snapshot.go)
  - Directory tree serialization: [`internal/archiver/tree.go`](https://github.com/restic/restic/blob/master/internal/archiver/tree.go)
  - Tree saver pipeline: [`internal/archiver/tree_saver.go`](https://github.com/restic/restic/blob/master/internal/archiver/tree_saver.go)
  - Repository locking: [`cmd/restic/lock.go`](https://github.com/restic/restic/blob/master/cmd/restic/lock.go)

#### Concrete Capabilities Relevant to SmartStrm

1. **Output Ownership and Manifests**:  
   *Verified Fact*: Restic establishes immutable output ownership using snapshot manifests ([`internal/data/snapshot.go`](https://github.com/restic/restic/blob/master/internal/data/snapshot.go)). Every backup produces a snapshot referencing a cryptographic tree root ID ([`internal/archiver/tree.go`](https://github.com/restic/restic/blob/master/internal/archiver/tree.go)). State ownership is 100% manifest-driven, verifiable, and immutable.

2. **Dry-Run and Execution Preview**:  
   *Verified Fact*: The `--dry-run` flag is available across `backup`, `forget`, and `prune` operations ([`cmd/restic/cmd_prune.go#L75`](https://github.com/restic/restic/blob/master/cmd/restic/cmd_prune.go#L75)). In prune, it reports exactly how many pack files, blobs, and bytes would be repacked or deleted without altering repository indexes or removing storage blobs.

3. **Reconciliation and Conservative Cleanup**:  
   *Verified Fact*: Restic implements a two-phase mark-and-sweep garbage collector in [`cmd/restic/cmd_prune.go`](https://github.com/restic/restic/blob/master/cmd/restic/cmd_prune.go). First, it loads all valid snapshot manifests and marks all referenced trees and data blobs. Next, it identifies unreferenced blobs and repackages partially used packs. Finally, it commits new indexes and safely removes unreferenced packs.  
   *Verified Fact*: Prune requires an exclusive repository lock (`openWithExclusiveLock`), preventing concurrent writes from corrupting data ([`cmd/restic/cmd_prune.go#L179`](https://github.com/restic/restic/blob/master/cmd/restic/cmd_prune.go#L179)).

4. **Rename and Recovery Semantics**:  
   *Verified Fact*: Restic uses content-defined chunking (Rabin fingerprints). When a file or directory is renamed, chunk IDs remain identical. Restic detects identical chunks across different paths and only stores new tree metadata, taking negligible space and zero blob duplication.

5. **Task Generation and Versioning**:  
   *Verified Fact*: Snapshots carry parent IDs, creation timestamps, hostnames, user tags, and path summaries, providing complete historical versioning of every state generation.

6. **Large-Directory Handling**:  
   *Verified Fact*: The archiver streams directory nodes in chunks using [`internal/archiver/tree_saver.go`](https://github.com/restic/restic/blob/master/internal/archiver/tree_saver.go). It avoids loading entire directory hierarchies into RAM, keeping memory usage proportional to directory depth rather than total item count.

7. **Media-Server Refresh**:  
   *Verified Fact*: Restic is a general backup program and does not interact with media servers.

8. **Safety Controls**:  
   *Verified Fact*: Restic enforces cryptographic integrity checks (`restic check`), append-only backend configurations, lock expiration timeouts, and tolerates configurable unused data limits (`--max-unused`) to minimize churn.

---

## 3. Comparative Capability Matrix

The following table compares SmartStrm's documented current state against all seven evaluated projects across the eight focus areas.

| Capability Focus Area | SmartStrm (Current State) | rclone | AList | MoviePilot | Auto_Symlink | openlist-strm | embyExternalUrl | restic |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **1. Output Ownership / Manifests** | **Signed Task Scope, No Ledger**: Cleanup verifies task ownership, but generation still uses an ephemeral `expected` map and has no persistent ledger | **Diff-Based / Manifests**: Compares remotes; supports `--files-from` and `--compare-dest` | **Virtual Database**: Tracks storage mounts and tasks in SQL; serves virtual paths | **Database Ledger**: `TransferHistory` table tracks source, dest, mode, size, and TMDB ID | **Filesystem Scans**: Checks target paths directly via `readlink` and HTTP requests | **SQLite Records**: `File` table records paths, hashes, sizes, and STRM targets | **In-Memory Cache**: NJS shared memory dicts; no disk files generated | **Immutable Manifests**: Snapshot trees link all generated blobs; 100% manifest ownership |
| **2. Dry-Run / Preview** | **Focused Previews**: Single-file generation preview plus cleanup scanned/planned/kept counts; no complete task-run plan | **Global Dry-Run**: `--dry-run` across sync, copy, move, and prune with detailed logs | **Queue Preview**: UI displays planned background transfer paths | **UI Scrape Preview**: Shows target paths, titles, and filenames before execution | **None**: Directly modifies filesystem during scheduled passes | **Cron Preview**: Dedicated `/api/preview/cron` handler tests scheduled generation | **Log-Level Preview**: Configurable NJS log level previews rewrites | **Global Dry-Run**: `--dry-run` across backup, forget, and prune |
| **3. Conservative Cleanup** | **Ownership Preview**: Signed task-scope checks, read-only count preview, and explicit confirmation; sync deletion still lacks backup dir or caps | **Gold Standard**: `--delete-after`, `--backup-dir`, and `--max-delete <N>` caps | **Trash Support**: Moves cloud files to provider recycle bin where available | **History-Driven**: Deletes only recorded outputs; supports recycle bin | **Direct Unlink**: `os.remove` and `shutil.rmtree` on dead targets; risk if unmounted | **Database Sync**: Deletes local files and removes matching SQLite rows | **Stateless**: No filesystem cleanup needed | **Two-Phase GC**: Mark-and-sweep prune with exclusive lock and index rebuild |
| **4. Rename / Recovery Semantics** | **Two-Stage Temp Rename**: Uses `.smartstrm-*` swap; rollback runs for up to 30 seconds after request cancellation and reports recovery failures, but has no durable recovery journal | **Hash Tracking**: `--track-renames` detects renamed files and issues server moves | **API Moves**: Drivers implement native remote rename and move | **Undo / Redo Tools**: Reverts transfers using database history | **Watcher Events**: Intercepts inotify `on_moved` to rename links in real time | **Path Updates**: Updates SQLite record and recreates STRM | **Dynamic Resolution**: Resolves current path on each stream request | **Deduplication**: Content-defined chunking reuses blobs across renames |
| **5. Task Scheduling & Versioning** | **Serial Queues**: 5-field cron, buffer-1 channels, watcher; runtime handover now freezes accepted work before replacing a manager, while durable task history remains limited | **Job Daemon**: Async jobs with IDs and status via `rclone rcd` | **Task Queue**: Worker pool with priority, retry, and persistence | **Event Chains**: APScheduler and event bus orchestrate tasks | **Thread Loop**: Thread-based sleep and cron intervals | **Cron per Mapping**: Independent `robfig/cron` tasks with rate limiters | **Periodic Tasks**: Nginx periodic timers for token and route refreshes | **Snapshot Trees**: Versioned snapshots with tags, parent IDs, and metadata |
| **6. Large-Directory Handling** | **16 MiB Response Cap**: OpenList 200-item paging; generator buffers entire lists in RAM | **Streaming Fast-List**: Paged directory streaming without RAM bloat | **Lazy Paging**: Cursor pagination and virtualized UI lists | **Generator Pipelines**: Yields files in chunks to conserve memory | **Recursive Walk**: Standard `os.walk`; latency spikes on fuse mounts | **Batch DB Insert**: Paged Alist reads and batch database inserts | **On-Demand**: Single-item resolution avoids directory scans entirely | **Chunked Trees**: Directory trees serialized in ~4MB pack files |
| **7. Media-Server Refresh** | **Inbound Webhook Only**: Receives Emby delete events; no outbound library refresh | **External Scripting**: Relies on downstream automation tools | **Mount Gateway**: Relies on external reverse proxy or scripts | **First-Class API**: Calls `refresh_library_by_path` on Emby, Jellyfin, and Plex | **API Refresh**: Triggers Emby and Jellyfin scan on file creation | **Webhook Alerts**: Generic webhooks; Emby refresh planned | **Inline Interception**: Rewrites `PlaybackInfo` and 302 URLs dynamically | **N/A**: General backup tool |
| **8. Safety Controls** | **Linux Descriptors**: `openat` / `unlinkat` root bounds; entry error isolation; HMAC URLs | **Comprehensive**: `--max-delete`, `--backup-dir`, `--read-only`, `OverlappingFilterCheck` | **RBAC & Signed URLs**: Role masks, signed download tokens, rate limits | **Size & Filter Bounds**: Minimum file size filters, regex excludes, trash routing | **Mount Check**: Confirms mount root exists before cleaning | **Concurrency Caps**: Per-task concurrency limits prevent API bans | **Automatic Fallback**: Falls back to original stream if 302 fails | **Cryptographic**: Exclusive locks, repository verification, read-only mode |

---

## 4. SmartStrm Gap Analysis Against Community Best Practices

Comparing SmartStrm against these projects highlights six concrete architectural gaps.

### Gap 1: Output Ownership and the Manifest Deficit (Critical)
- **Documented Current State**: SmartStrm's generator builds an ephemeral in-memory set (`expected := make(map[string]struct{})`) during a single task run ([`internal/task/generator.go:765`](../../internal/task/generator.go)). In `removeStale`, it walks the destination directory and deletes any `.strm` or copied file not present in that map.
- **Problem**: R02 and R03 led to output-overlap validation and signed task-scope checks, so the current toolbox no longer blindly removes unrelated files. The unresolved architectural problem is historical ownership: without a durable ledger, SmartStrm cannot prove which earlier task definition created a path or efficiently reconcile outputs after task configuration changes.
- **Community Pattern**: MoviePilot and openlist-strm record every generated output in an embedded database ledger. Restic binds every output to immutable snapshot trees. Ownership is explicit, persistent, and task-scoped.

### Gap 2: Destructive Cleanup Without Safety Nets (High)
- **Documented Current State**: SmartStrm's `removeStale` calls `pathpolicy.RemoveFileAt` immediately on unexpected files. If any entry fails during traversal, sync deletion aborts, which provides good baseline protection. Toolbox cleanup now exposes a read-only count preview and independently revalidates signed task ownership during deletion. However, if remote cloud storage returns an empty directory list (for example, session expiration or temporary API failure), automated sync deletion can still treat the traversal as successful and unlink every generated local `.strm` file.
- **Problem**: There is no threshold cap on deletions and no recovery directory. Once unlinked, files are gone permanently.
- **Community Pattern**: Rclone protects users with `--max-delete <N>` (aborting if deletions exceed an expected limit) and `--backup-dir` (moving obsolete files to a backup staging area instead of permanently deleting them).

### Gap 3: Narrow Dry-Run Simulation (Medium)
- **Documented Current State**: SmartStrm provides a single-file generation preview endpoint (`/api/tasks/{id}/preview`) and a cleanup preview (`GET /api/tasks/{id}/generated`) reporting scanned, planned, removed, and kept counts without modifying files.
- **Problem**: Operators still cannot preview a complete task run. They cannot see the full set of files that will be created, modified, copied, or removed by generation and sync deletion. The October review's R13 directory-level replacement mismatch is repaired for the covered preview path, but there is still no shared complete task-run plan.
- **Community Pattern**: Rclone and restic provide complete dry-run modes (`--dry-run`) that evaluate all filters and directory walks, emitting a complete change plan without touching disk.

### Gap 4: Fragile Batch Rename Recovery (Medium)
- **Documented Current State**: SmartStrm's batch rename engine ([`internal/rename/rename.go`](../../internal/rename/rename.go)) uses a two-stage transaction with temporary suffixes (`.smartstrm-*`). Rollback now uses a cancellation-independent context bounded to 30 seconds and joins recovery failures into the returned error.
- **Problem**: Recovery still has no durable journal or resumable status. If rollback itself cannot complete before its deadline or a storage operation keeps failing, temporary files can remain without an operator-visible recovery record.
- **Community Pattern**: Rclone tracks renames through hash-and-size matching (`--track-renames`), while MoviePilot records reversible transfer history entries with dedicated redo/undo tools.

### Gap 5: Missing Outbound Media-Server Refresh (Medium)
- **Documented Current State**: SmartStrm receives inbound Emby delete events to remove remote source files, and its media proxy rewrites Emby/Jellyfin `PlaybackInfo` calls. However, outbound library refresh is completely absent (`docs/FEATURE_MATRIX.md` marks 飞牛影视刷新 as unimplemented ⬜).
- **Problem**: When SmartStrm generates new `.strm` files, media servers remain unaware of them until their next scheduled library scan or until the user manually triggers a rescan.
- **Community Pattern**: MoviePilot and Auto_Symlink call media server APIs immediately upon file creation, targeting only the modified library path (`refresh_library_by_path`).

### Gap 6: Large-Directory Buffering and UI Virtualization (Low to Medium)
- **Documented Current State**: SmartStrm enforces a healthy 16 MiB limit on remote metadata responses and uses 200-item paging for OpenList. However, `Generator.Generate` still collects full directory slices in memory, and the frontend `StorageBrowserPage.tsx` renders file tables with `pagination=false` and no virtual list.
- **Problem**: Browsing cloud directories containing 10,000+ files causes frontend DOM lag and backend memory spikes.
- **Community Pattern**: AList and Rclone stream directory listings in chunks and render browser tables with virtualized scrolling.

---

## 5. Three Implementable Candidate Improvements

### Candidate 1: Task-Level Output Manifest and Ownership Tracking

#### Description
Introduce an embedded, task-scoped output manifest ledger within SmartStrm. Each time a task generates or copies a file, it records an entry with:
- `task_id`: The owning task identifier.
- `storage_id`: The source storage identifier.
- `source_path`: The remote file path.
- `destination_path`: The local `.strm` or copied asset path.
- `file_size`: The media file size.
- `generated_at`: Unix timestamp of generation.
- `content_hash`: SHA-256 hash of the generated `.strm` payload.

This can be implemented either as an embedded SQLite table using a pure Go driver (such as `modernc.org/sqlite`) or as an atomic JSON-Lines file located at `runtime/manifests/{task_id}.jsonl`.

#### Concrete Benefits
- **Makes Ownership Durable**: The cleanup phase in `removeStale` only removes files that belong to the current task's previous manifest. This reinforces the existing overlap checks with historical task ownership.
- **Strengthens Toolbox Proof**: The toolbox action "Clear Generated Files" can validate both the signed URL and a manifest record, preserving original media files and user-created assets even after task definitions change.
- **Enables True Incremental Diffs**: Future task runs can compare the remote directory against the recorded manifest without re-reading every `.strm` file from disk.

#### Implementation Scope in Go Monolith
Add a new package `internal/manifest/` exposing a simple interface:
```go
type Manifest interface {
    Record(ctx context.Context, entry Entry) error
    ListOwned(ctx context.Context, taskID string) ([]Entry, error)
    Prune(ctx context.Context, taskID string, currentExpected map[string]struct{}) ([]string, error)
}
```
This integrates cleanly into `internal/task/generator.go` without altering the storage driver abstractions.

---

### Candidate 2: Conservative Deletion Safeguards (Max-Delete Cap and Backup Staging)

#### Description
Adopt rclone's proven deletion safety guards by adding two optional controls to `config.TaskConfig`:
1. `max_delete_limit` (integer or percentage string, for example `50` or `10%`): SmartStrm calculates the total number of files marked for deletion before unlinking anything. If `len(staleFiles) > max_delete_limit`, the cleanup phase aborts immediately, logs a critical alert to the history store, and leaves all files intact.
2. `backup_dir` (string path): When configured, obsolete `.strm` and copied files are moved into a timestamped subfolder under `backup_dir` (for example `runtime/backups/{task_id}/2026-11-05_120000/`) instead of calling `pathpolicy.RemoveFileAt`.

#### Concrete Benefits
- **Zero Accidental Mass Deletions**: Protects users when a cloud drive token expires or returns an empty file list.
- **Instant Rollback**: If an operator unintentionally changes a filter regex or path rule, all deleted `.strm` files can be restored directly from the backup directory.

#### Implementation Scope in Go Monolith
This change touches only `internal/task/generator.go` around `removeStale` and adds configuration fields in `internal/config/config.go`. It requires zero third-party dependencies and under 150 lines of Go code.

---

### Candidate 3: Outbound Media-Server Path-Level Refresh Integration

#### Description
Build a lightweight media server client package in `internal/mediaserver/` that supports Emby, Jellyfin, and Plex APIs. When a generation task finishes with `result.Created > 0` or `result.Removed > 0`, SmartStrm invokes the configured media server client to refresh only the affected destination folder.

#### Concrete Benefits
- **Immediate Media Availability**: New episodes and movies become playable in Emby/Jellyfin within seconds of generation, eliminating the need to wait for hourly background library scans.
- **Minimal Resource Overhead**: Calling `/Library/Media/Updated` with specific folder paths consumes a fraction of the CPU and I/O required for full media library scans.

#### Implementation Scope in Go Monolith
Create `internal/mediaserver/client.go` with an interface:
```go
type Client interface {
    RefreshPath(ctx context.Context, localPath string) error
}
```
Hook this client into `internal/task/manager.go` at the end of successful task execution.

---

## 6. Evidence-Based Recommendation

### Immediate Recommendation: Preview Then Confirm With Independent Revalidation

Expose the existing task ownership checks as authenticated `GET /api/tasks/{id}/generated`, report scanned/planned/removed/kept counts, and require the React task-tools UI to obtain that preview before enabling `DELETE`. Execution must independently re-read each STRM and apply the same ownership predicate again, so files that no longer satisfy the current task scope are retained. The preview is advisory rather than a locked plan, so execution counts may change if files are added or modified before confirmation. This is the smallest compatible implementation of the dry-run pattern demonstrated by rclone, restic, and openlist-strm.

### Long-Term Highest-Value Change: Candidate 1 Combined With Candidate 2's Safety Cap

Between the three candidates, **Candidate 1 is the highest-value architectural improvement for SmartStrm**, enhanced by the safety threshold from Candidate 2.

#### Detailed Justification

1. **Completes the Ownership Model Beyond Current R02 and R03 Protections**:  
   The October 2026 project review identified R02 and R03 as P1 severity issues. Current overlap validation and signed task-scope cleanup address the immediate unsafe behaviors. A manifest addresses the remaining architectural limitation: SmartStrm still cannot prove historical ownership after task definitions, source mappings, or signing keys change.

2. **Aligns With Proven Community Best Practices**:  
   Every mature tool surveyed solves this problem at the ownership layer:
   - Restic relies on snapshot trees so it never deletes unreferenced blobs without proof.
   - MoviePilot logs every output path in its transfer database before finalizing operations.
   - Openlist-strm uses its SQLite `files` table to tie each `.strm` file to its task.

3. **High Compatibility With the Go Monolith**:  
   SmartStrm is intentionally designed as a single Go binary with zero external service dependencies. A manifest module can be implemented using an atomic append-only JSON-Lines file per task or a lightweight SQLite database. Both options keep deployment trivial, require no external database server, and maintain container startup speeds.

4. **Lays the Foundation for Full Dry-Run and Fast Incremental Runs**:  
   Once SmartStrm maintains a durable manifest of what it owns, it can compare incoming remote directory listings against the manifest without touching the local disk. This makes dry-run previews fast, accurate, and completely side-effect free.

#### Step-by-Step Implementation Roadmap

1. **Phase 1: Manifest Package Definition (`internal/manifest/`)**  
   Create an interface that records `task_id`, `source_path`, `destination_path`, and `generated_at`. Write tests verifying atomic saves, crash resilience, and cross-platform file locking.

2. **Phase 2: Integrate Generator Output Recording**  
   In `internal/task/generator.go`, update `Generate` to record each newly written `.strm` and copied file into the active task manifest.

3. **Phase 3: Refactor `removeStale` to Reconcile Against Manifest**  
   Update `removeStale` so it inspects only files recorded in the task's previous manifest. If a file exists on disk and in the previous manifest, but is absent from the current run's expected set, it is marked for removal. Any file on disk not in the task's manifest is ignored.

4. **Phase 4: Add `max_delete_limit` Safety Gate**  
   Before executing the removals identified in Phase 3, verify that the total number of deletions does not exceed the task's configured threshold. If exceeded, abort with an error and preserve all files.

5. **Phase 5: Expose Manifest in Management UI**  
   Provide a read-only API endpoint (`/api/tasks/{id}/manifest`) allowing operators to inspect all files owned by a given task directly in the web interface.

---

## 7. Verification and Primary Source Citation Ledger

### 7.1 Primary Source Reference Ledger

| Project | Official Repository | Evaluated Version / Tag | Key Primary Source URLs |
| --- | --- | --- | --- |
| **rclone** | [rclone/rclone](https://github.com/rclone/rclone) | `v1.75.2` | [Sync Engine Source](https://github.com/rclone/rclone/blob/master/fs/sync/sync.go), [Track Renames Source](https://github.com/rclone/rclone/blob/master/fs/sync/trackrenames/trackrenames.go), [Operations Flags](https://github.com/rclone/rclone/blob/master/fs/operations/operationsflags/operationsflags.go) |
| **AList** | [alist-org/alist](https://github.com/alist-org/alist) | `v3.66.0` | [Task Manager Source](https://github.com/alist-org/alist/blob/main/pkg/task/manager.go), [115 Driver](https://github.com/alist-org/alist/blob/main/drivers/115_open/driver.go), [Driver Base](https://github.com/alist-org/alist/blob/main/drivers/base/driver.go) |
| **MoviePilot** | [jxxghp/MoviePilot](https://github.com/jxxghp/MoviePilot) | `v3.2.4` | [TransferHistory Model](https://github.com/jxxghp/MoviePilot/blob/v2/app/db/models/transferhistory.py), [Transfer Chain](https://github.com/jxxghp/MoviePilot/blob/v2/app/chain/transfer.py), [Media Server Helper](https://github.com/jxxghp/MoviePilot/blob/v2/app/helper/mediaserver.py) |
| **Auto_Symlink** | [shenxianmq/Auto_Symlink](https://github.com/shenxianmq/Auto_Symlink) | `0.1.1` | [SymlinkChecker Source](https://github.com/berry8838/Auto_Symlink_copy/blob/main/autosync/SymlinkChecker.py), [FileWatcher Source](https://github.com/berry8838/Auto_Symlink_copy/blob/main/watcher/FileWatcher.py), [Task Runner](https://github.com/berry8838/Auto_Symlink_copy/blob/main/task_run.py) |
| **openlist-strm** | [konghanghang/openlist-strm](https://github.com/konghanghang/openlist-strm) | `v0.1.0` | [GORM Storage Models](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/storage/models.go), [Generator Source](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/strm/generator.go), [Scheduler Source](https://github.com/konghanghang/openlist-strm/blob/master/backend/internal/scheduler/scheduler.go) |
| **embyExternalUrl** | [bpking1/embyExternalUrl](https://github.com/bpking1/embyExternalUrl) | `v0.4.8` | [Emby NJS Router](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/emby.js), [PlaybackInfo Module](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/modules/emby-playback-info.js), [Common Constants](https://github.com/bpking1/embyExternalUrl/blob/main/emby2Alist/nginx/conf.d/config/constant-common.js) |
| **restic** | [restic/restic](https://github.com/restic/restic) | `v0.19.1` | [Prune Engine Source](https://github.com/restic/restic/blob/master/cmd/restic/cmd_prune.go), [Snapshot Model](https://github.com/restic/restic/blob/master/internal/data/snapshot.go), [Repository Lock](https://github.com/restic/restic/blob/master/cmd/restic/lock.go) |

### 7.2 Fact Versus Inference Classification

1. **Verified Source Facts**:
   - Rclone deferral of deletions via `--delete-after` and backup isolation via `--backup-dir` are verified by `fs/sync/sync.go` line 849 and line 245.
   - MoviePilot's transfer history table schema is verified by `app/db/models/transferhistory.py` lines 25-60.
   - Openlist-strm's GORM `File` and `Task` models are verified by `backend/internal/storage/models.go` lines 8-45.
   - EmbyExternalUrl's `fallbackUseOriginal` fallback logic is verified by `emby2Alist/nginx/conf.d/emby.js` lines 54-65.
   - Restic's exclusive lock requirement for prune is verified by `cmd/restic/cmd_prune.go` line 179.
   - SmartStrm's current lack of a persistent manifest and in-memory expected map are verified by `internal/task/generator.go` lines 760-800 and `docs/FEATURE_MATRIX.md`.

2. **Inferred Conclusions**:
   - Auto_Symlink's `shutil.rmtree` directory deletion presents risk during cloud mount disconnects because it checks only parent path existence without atomic file verification.
   - Candidate 1 (manifest ledger) has higher return on investment than Candidate 3 (media server refresh) because it resolves data loss risks (R02, R03) that threaten data integrity.
   - Storing task manifests as local JSON-Lines files or embedded SQLite maintains zero-dependency deployment goals for SmartStrm's Go single binary.
