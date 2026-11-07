import { useEffect, useMemo, useRef, useState } from "react";
import {
	App,
	Button,
	Empty,
	Input,
	Modal,
	Popconfirm,
	Select,
	Space,
	Spin,
	Table,
	Tooltip,
	Typography,
} from "antd";
import type { TableProps } from "antd";
import {
	DeleteOutlined,
	DragOutlined,
	EditOutlined,
	LinkOutlined,
} from "@ant-design/icons";
import { useNavigate } from "react-router";
import { formatTimestamp } from "../format";
import { deleteJSON, getJSON, postJSON } from "../api/client";
import { getBootstrap } from "../api/bootstrap";
import type { MatchingTask, StorageEntry, StorageSummary } from "../api/types";
import ContextMenu from "../components/ContextMenu";
import EntryIcon, { isMediaEntry } from "../components/EntryIcon";
import BatchRenameModal from "./BatchRenameModal";
import TmdbModal from "./TmdbModal";
import "./StorageBrowserPage.css";

export const collator = new Intl.Collator("zh", {
	numeric: true,
	sensitivity: "base",
});

type SortKey = "name" | "size" | "modified";
type SortState = { key: SortKey; direction: "asc" | "desc" };

const STORAGE_TYPE_LABELS: Record<string, string> = {
	local: "本地",
	webdav: "WebDAV",
};

export function filterEntries(
	entries: StorageEntry[],
	filter: string,
): StorageEntry[] {
	const q = filter.trim().toLowerCase();
	return q ? entries.filter((e) => e.name.toLowerCase().includes(q)) : entries;
}

export function sortEntries(
	entries: StorageEntry[],
	sort: SortState,
): StorageEntry[] {
	const factor = sort.direction === "asc" ? 1 : -1;
	return [...entries].sort((a, b) => {
		if (sort.key === "name") return factor * collator.compare(a.name, b.name);
		if (sort.key === "size") return factor * (a.size - b.size);
		return (
			factor * (Date.parse(a.modified || "0") - Date.parse(b.modified || "0"))
		);
	});
}

// 与旧 internal/app/ui/index.html 的 formatSize 完全一致。
export function formatSize(n: number): string {
	if (!n) return "—";
	const units = ["B", "KB", "MB", "GB", "TB"];
	let i = 0;
	let v = n;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return v.toFixed(i ? 1 : 0) + " " + units[i];
}

const NAME_WIDTH_KEY = "smartstrm.storageNameWidth";
const NAME_WIDTHS = [220, 320, 420];

function loadNameWidth(): number {
	try {
		const v = Number(localStorage.getItem(NAME_WIDTH_KEY));
		return NAME_WIDTHS.includes(v) ? v : 320;
	} catch {
		return 320;
	}
}

function parentPath(path: string): string {
	// 与旧 UI 的 up() 一致：去尾段，根停住。
	const parts = path.replace(/\/$/, "").split("/");
	parts.pop();
	return parts.join("/") || "/";
}

function RenameModal({
	entry,
	loading,
	onCancel,
	onOk,
}: {
	entry: StorageEntry | null;
	loading: boolean;
	onCancel: () => void;
	onOk: (name: string) => void;
}) {
	const [name, setName] = useState(entry?.name ?? "");
	return (
		<Modal
			title="重命名"
			open={entry !== null}
			onCancel={onCancel}
			onOk={() => {
				if (name.trim()) onOk(name.trim());
			}}
			confirmLoading={loading}
			okText="确定"
			cancelText="取消"
		>
			<Input
				aria-label="新名称"
				value={name}
				onChange={(e) => setName(e.target.value)}
				placeholder="新名称"
			/>
		</Modal>
	);
}

function MoveModal({
	entry,
	loading,
	onCancel,
	onOk,
}: {
	entry: StorageEntry | null;
	loading: boolean;
	onCancel: () => void;
	onOk: (destination: string) => void;
}) {
	const [destination, setDestination] = useState("/");
	return (
		<Modal
			title={entry ? `移动 ${entry.name}` : "移动"}
			open={entry !== null}
			onCancel={onCancel}
			onOk={() => {
				if (destination.trim()) onOk(destination.trim());
			}}
			confirmLoading={loading}
			okText="确定"
			cancelText="取消"
		>
			<Input
				aria-label="目标目录"
				value={destination}
				onChange={(e) => setDestination(e.target.value)}
				placeholder="目标目录"
			/>
		</Modal>
	);
}

function MkdirModal({
	open,
	loading,
	onCancel,
	onOk,
}: {
	open: boolean;
	loading: boolean;
	onCancel: () => void;
	onOk: (name: string) => void;
}) {
	const [name, setName] = useState("");
	return (
		<Modal
			title="新建目录"
			open={open}
			onCancel={onCancel}
			onOk={() => {
				if (name.trim()) onOk(name.trim());
			}}
			confirmLoading={loading}
			okText="确定"
			cancelText="取消"
		>
			<Input
				aria-label="目录名"
				value={name}
				onChange={(e) => setName(e.target.value)}
				placeholder="目录名"
			/>
		</Modal>
	);
}

export default function StorageBrowserPage() {
	const { message } = App.useApp();
	const navigate = useNavigate();
	const [storages, setStorages] = useState<StorageSummary[]>([]);
	const [storagesLoading, setStoragesLoading] = useState(true);
	const [storageId, setStorageId] = useState("");
	const [path, setPath] = useState("/");
	const [pathInput, setPathInput] = useState("/");
	const [entries, setEntries] = useState<StorageEntry[]>([]);
	const [browsing, setBrowsing] = useState(false);
	const [browseReady, setBrowseReady] = useState(false);
	const [filter, setFilter] = useState("");
	const [sort, setSort] = useState<SortState>({
		key: "name",
		direction: "asc",
	});
	const [nameWidth, setNameWidth] = useState(loadNameWidth);
	const [actionLoading, setActionLoading] = useState<string | null>(null);
	const [renameTarget, setRenameTarget] = useState<StorageEntry | null>(null);
	const [moveTarget, setMoveTarget] = useState<StorageEntry | null>(null);
	const [mkdirOpen, setMkdirOpen] = useState(false);
	const [menu, setMenu] = useState<{
		x: number;
		y: number;
		entry: StorageEntry;
	} | null>(null);
	const [matchingTasks, setMatchingTasks] = useState<MatchingTask[]>([]);
	const [batchRenameOpen, setBatchRenameOpen] = useState(false);
	const [tmdbOpen, setTmdbOpen] = useState(false);
	// 对应旧 UI 的 tmdbConfigured 门控；bootstrap 获取失败时保持可用，不阻断用户。
	const [tmdbConfigured, setTmdbConfigured] = useState(true);
	// browseSeq：并发 browse 最新者胜，陈旧响应不得覆盖更新的 entries/path。
	const browseSeq = useRef(0);
	// navRef 同步记录当前位置，对应旧 UI 的 pathInput.value / storageSelect.value，
	// 供 matching-tasks 响应返回时做陈旧守卫。
	const navRef = useRef({ storageId: "", path: "/" });

	// 与旧 loadCurrentTasks 一致：先隐藏按钮，响应返回时若存储/路径已变则丢弃，失败保持隐藏。
	const loadCurrentTasks = async (remotePath: string, id: string) => {
		setMatchingTasks([]);
		try {
			const matches = await getJSON<MatchingTask[]>(
				`/api/storages/${encodeURIComponent(id)}/matching-tasks?path=${encodeURIComponent(remotePath)}`,
			);
			const current = navRef.current;
			if (current.path !== remotePath || current.storageId !== id) return;
			setMatchingTasks(matches);
		} catch {
			// 失败时保持“运行当前”隐藏（旧行为）
		}
	};

	const browse = async (nextPath: string, id = storageId) => {
		if (!id) return;
		const seq = ++browseSeq.current;
		setBrowsing(true);
		setBrowseReady(false);
		setEntries([]);
		setMatchingTasks([]);
		setMenu(null);
		setRenameTarget(null);
		setMoveTarget(null);
		setMkdirOpen(false);
		setBatchRenameOpen(false);
		setTmdbOpen(false);
		try {
			const list = await getJSON<StorageEntry[]>(
				`/api/storages/${encodeURIComponent(id)}/entries?path=${encodeURIComponent(nextPath)}`,
			);
			if (seq !== browseSeq.current) return; // 已有更新的浏览请求，丢弃陈旧响应
			navRef.current = { storageId: id, path: nextPath };
			setStorageId(id);
			setEntries(list);
			setPath(nextPath);
			setPathInput(nextPath);
			setBrowseReady(true);
			void loadCurrentTasks(nextPath, id);
		} catch (err) {
			if (seq !== browseSeq.current) return;
			message.error((err as Error).message);
		} finally {
			if (seq === browseSeq.current) setBrowsing(false);
		}
	};

	// 与旧 runCurrentDirectory 一致：始终运行第一个匹配任务，body 携带当前目录路径。
	const runCurrent = async () => {
		const task = matchingTasks[0];
		if (!task || !browseReady || browsing) return;
		try {
			await postJSON(`/api/tasks/${encodeURIComponent(task.id)}/run`, { path });
			message.success(`已将 ${task.name || task.id} 的当前目录加入队列`);
		} catch (err) {
			message.error((err as Error).message);
		}
	};

	useEffect(() => {
		void (async () => {
			try {
				const list = await getJSON<StorageSummary[]>("/api/storages");
				setStorages(list);
				if (list.length > 0) {
					setStorageId(list[0].id);
					await browse("/", list[0].id);
				}
			} catch (err) {
				message.error((err as Error).message);
			} finally {
				setStoragesLoading(false);
			}
		})();
		void (async () => {
			try {
				const bootstrap = await getBootstrap();
				setTmdbConfigured(bootstrap.tmdb_configured);
			} catch {
				// 获取失败保持按钮可用（不由前端额外提示）
			}
		})();
		// 仅挂载时执行一次
	}, []);

	const filtered = useMemo(
		() => filterEntries(entries, filter),
		[entries, filter],
	);
	const sorted = useMemo(() => sortEntries(filtered, sort), [filtered, sort]);

	if (storagesLoading) {
		return (
			<Spin tip="正在加载存储">
				<div style={{ minHeight: 160 }} />
			</Spin>
		);
	}

	if (storages.length === 0 && storageId === "") {
		return (
			<section className="management-page">
				<header className="management-heading">
					<div>
						<h2>存储浏览</h2>
						<Typography.Text type="secondary">
							浏览云盘或本机目录中的媒体文件
						</Typography.Text>
					</div>
				</header>
				<Empty description="尚未添加存储。先连接媒体文件所在的位置，才能在这里浏览文件。">
					<Button
						type="primary"
						onClick={() => navigate("/config?section=storages")}
					>
						前往添加存储
					</Button>
				</Empty>
			</section>
		);
	}

	const onTableChange: TableProps<StorageEntry>["onChange"] = (
		_pagination,
		_filters,
		sorter,
	) => {
		const s = Array.isArray(sorter) ? sorter[0] : sorter;
		if (s.columnKey === "name" && s.order) {
			setSort({
				key: "name",
				direction: s.order === "descend" ? "desc" : "asc",
			});
		}
	};

	const changeNameWidth = (value: number) => {
		setNameWidth(value);
		try {
			localStorage.setItem(NAME_WIDTH_KEY, String(value));
		} catch {
			// localStorage 不可用时忽略
		}
	};

	const submitRename = async (name: string) => {
		const entry = renameTarget;
		if (!entry || actionLoading || !browseReady || browsing) return;
		const seq = browseSeq.current;
		setActionLoading(`${entry.path}:rename`);
		try {
			await postJSON(`/api/storages/${encodeURIComponent(storageId)}/rename`, {
				path: entry.path,
				new_name: name,
			});
			message.success("已重命名");
			setRenameTarget(null);
			if (seq === browseSeq.current) await browse(path, storageId);
		} catch (err) {
			message.error((err as Error).message);
		} finally {
			setActionLoading(null);
		}
	};

	const submitMove = async (destination: string) => {
		const entry = moveTarget;
		if (!entry || actionLoading || !browseReady || browsing) return;
		const seq = browseSeq.current;
		setActionLoading(`${entry.path}:move`);
		try {
			await postJSON(`/api/storages/${encodeURIComponent(storageId)}/move`, {
				path: entry.path,
				destination,
			});
			message.success("已移动");
			setMoveTarget(null);
			if (seq === browseSeq.current) await browse(path, storageId);
		} catch (err) {
			message.error((err as Error).message);
		} finally {
			setActionLoading(null);
		}
	};

	const submitDelete = async (entry: StorageEntry) => {
		if (actionLoading || !browseReady || browsing) return;
		const seq = browseSeq.current;
		setActionLoading(`${entry.path}:delete`);
		try {
			await deleteJSON(
				`/api/storages/${encodeURIComponent(storageId)}/entry?path=${encodeURIComponent(entry.path)}`,
			);
			message.success("已删除");
			if (seq === browseSeq.current) await browse(path, storageId);
		} catch (err) {
			message.error((err as Error).message);
		} finally {
			setActionLoading(null);
		}
	};

	const copyURL = async (entry: StorageEntry) => {
		if (actionLoading || !browseReady || browsing) return;
		setActionLoading(`${entry.path}:copy`);
		try {
			// 与旧 UI 的 copyURL 一致：直接写入后端返回的 url。
			const d = await getJSON<{ url: string }>(
				`/api/storages/${encodeURIComponent(storageId)}/stream-url?path=${encodeURIComponent(entry.path)}`,
			);
			await navigator.clipboard.writeText(d.url);
			message.success("STRM 地址已复制");
		} catch (err) {
			message.error((err as Error).message);
		} finally {
			setActionLoading(null);
		}
	};

	const submitMkdir = async (name: string) => {
		if (actionLoading || !browseReady || browsing) return;
		const seq = browseSeq.current;
		setActionLoading(`${path}:mkdir`);
		try {
			// mkdir 处理器只读 path 字段，与旧 UI 一样发送拼接后的完整路径。
			await postJSON(`/api/storages/${encodeURIComponent(storageId)}/mkdir`, {
				path: path.replace(/\/$/, "") + "/" + name,
			});
			message.success("已创建");
			setMkdirOpen(false);
			if (seq === browseSeq.current) await browse(path, storageId);
		} catch (err) {
			message.error((err as Error).message);
		} finally {
			setActionLoading(null);
		}
	};

	const columns: TableProps<StorageEntry>["columns"] = [
		{
			title: "名称",
			key: "name",
			width: nameWidth,
			ellipsis: true,
			sorter: true,
			sortOrder:
				sort.key === "name"
					? sort.direction === "asc"
						? "ascend"
						: "descend"
					: null,
			render: (_, entry) => (
				<Space size={4}>
					<EntryIcon entry={entry} />
					{entry.is_dir ? (
						<Typography.Link strong onClick={() => void browse(entry.path)}>
							{entry.name}
						</Typography.Link>
					) : (
						entry.name
					)}
				</Space>
			),
		},
		{
			title: "类型",
			key: "type",
			width: 88,
			render: (_, entry) => (entry.is_dir ? "目录" : "文件"),
		},
		{
			title: "大小",
			key: "size",
			width: 110,
			align: "right",
			render: (_, entry) => formatSize(entry.size),
		},
		{
			title: "修改时间",
			key: "modified",
			width: 180,
			render: (_, entry) => (
				<span className="storage-time">{formatTimestamp(entry.modified)}</span>
			),
		},
		{
			title: "操作",
			key: "actions",
			width: 224,
			fixed: "right",
			render: (_, entry) => (
				<Space size={4} className="storage-row-actions">
					<Tooltip title="重命名">
						<Button
							type="text"
							icon={<EditOutlined />}
							aria-label="重命名"
							disabled={!browseReady || browsing || !!actionLoading}
							loading={actionLoading === `${entry.path}:rename`}
							onClick={() => setRenameTarget(entry)}
						/>
					</Tooltip>
					<Tooltip title="移动">
						<Button
							type="text"
							icon={<DragOutlined />}
							aria-label="移动"
							disabled={!browseReady || browsing || !!actionLoading}
							loading={actionLoading === `${entry.path}:move`}
							onClick={() => setMoveTarget(entry)}
						/>
					</Tooltip>
					{!entry.is_dir && (
						<Tooltip title="复制 STRM 地址">
							<Button
								type="text"
								icon={<LinkOutlined />}
								aria-label="复制STRM地址"
								disabled={!browseReady || browsing || !!actionLoading}
								loading={actionLoading === `${entry.path}:copy`}
								onClick={() => void copyURL(entry)}
							/>
						</Tooltip>
					)}
					<Popconfirm
						title={`确定删除 ${entry.name}？此操作不可恢复。`}
						okText="删除"
						cancelText="取消"
						okButtonProps={{ danger: true }}
						onConfirm={() => void submitDelete(entry)}
					>
						<Tooltip title="删除">
							<Button
								type="text"
								danger
								icon={<DeleteOutlined />}
								aria-label="删除"
								disabled={!browseReady || browsing || !!actionLoading}
								loading={actionLoading === `${entry.path}:delete`}
							/>
						</Tooltip>
					</Popconfirm>
				</Space>
			),
		},
	];

	return (
		<section className="management-page">
			<header className="management-heading">
				<div>
					<h2>存储浏览</h2>
					<Typography.Text type="secondary">
						浏览云盘目录并管理文件与 STRM 地址
					</Typography.Text>
				</div>
			</header>
			<div className="storage-toolbar">
				<div
					className="storage-toolbar-group storage-location"
					role="group"
					aria-label="位置导航"
				>
					<Select
						disabled={actionLoading !== null}
						aria-label="存储"
						placeholder="存储"
						value={storageId || undefined}
						options={storages.map((s) => ({
							value: s.id,
						label: `${s.id} (${STORAGE_TYPE_LABELS[s.type] ?? s.type})`,
						}))}
						onChange={(id) => {
							// 存储立即切换（与旧 storageSelect.value 语义一致），browse 成功后 navRef 再整体更新
							navRef.current = { storageId: id, path: navRef.current.path };
							setStorageId(id);
							void browse("/", id);
						}}
					/>
					<Input
						aria-label="路径"
						value={pathInput}
						onChange={(e) => setPathInput(e.target.value)}
						onPressEnter={() => void browse(pathInput || "/")}
					/>
					<Button onClick={() => void browse(pathInput || "/")}>前往</Button>
					<Button onClick={() => void browse(parentPath(path))}>上一级</Button>
				</div>
				<div
					className="storage-toolbar-group storage-display"
					role="group"
					aria-label="筛选和显示"
				>
					<Input
						aria-label="筛选文件"
						placeholder="筛选"
						allowClear
						value={filter}
						onChange={(e) => setFilter(e.target.value)}
					/>
					<Select
						aria-label="名称列宽"
						value={nameWidth}
						options={[
							{ value: 220, label: "紧凑 220" },
							{ value: 320, label: "标准 320" },
							{ value: 420, label: "加宽 420" },
						]}
						onChange={changeNameWidth}
					/>
				</div>
				<div
					className="storage-toolbar-group storage-actions"
					role="group"
					aria-label="目录操作"
				>
					{matchingTasks.length > 0 && (
						<Button type="primary" onClick={() => void runCurrent()}>
							运行当前
						</Button>
					)}
					<Button disabled={!browseReady || browsing} onClick={() => setMkdirOpen(true)}>新建目录</Button>
					<Button disabled={!browseReady || browsing} onClick={() => setBatchRenameOpen(true)}>批量命名</Button>
					{/* 与旧 UI 的 TMDB 门控一致：未配置 API Key 时禁用并提示 */}
					<Tooltip
						title={
							tmdbConfigured
								? undefined
								: "TMDB API Key 未配置，请在配置页设置后重启服务"
						}
					>
						<Button
							disabled={!tmdbConfigured || !browseReady || browsing}
							onClick={() => setTmdbOpen(true)}
						>
							影视识别
						</Button>
					</Tooltip>
				</div>
				<div className="storage-toolbar-status">
					<span role="status" aria-live="polite">
						显示 {filtered.length} / 共 {entries.length} 项
					</span>
					{!tmdbConfigured && (
						<Typography.Text type="secondary">
							未配置 TMDB API Key
						</Typography.Text>
					)}
				</div>
			</div>
			<Table<StorageEntry>
				loading={browsing}
				className="storage-table"
				rowKey="path"
				pagination={false}
				columns={columns}
				dataSource={sorted}
				locale={{
					emptyText: (
						<Typography.Text type="secondary">当前目录为空</Typography.Text>
					),
				}}
				scroll={{ x: nameWidth + 602 }}
				onChange={onTableChange}
				onRow={(entry) => {
					// 与旧 UI 一致：仅媒体行有右键菜单、tabIndex 与 aria-haspopup
					if (!isMediaEntry(entry)) return {};
					return {
						tabIndex: 0,
						"aria-haspopup": "menu",
						onContextMenu: (event) => {
							event.preventDefault();
							setMenu({ x: event.clientX, y: event.clientY, entry });
						},
						onKeyDown: (event) => {
							if (
								event.key === "ContextMenu" ||
								(event.key === "F10" && event.shiftKey)
							) {
								event.preventDefault();
								// 旧 UI 键盘打开时定位在行 rect.left+20 / rect.top+20
								const rect = event.currentTarget.getBoundingClientRect();
								setMenu({ x: rect.left + 20, y: rect.top + 20, entry });
							}
						},
					};
				}}
			/>
			<RenameModal
				key={"rename:" + (renameTarget?.path ?? "")}
				entry={renameTarget}
				loading={
					renameTarget !== null &&
					actionLoading === `${renameTarget.path}:rename`
				}
				onCancel={() => setRenameTarget(null)}
				onOk={(name) => void submitRename(name)}
			/>
			<MoveModal
				key={"move:" + (moveTarget?.path ?? "")}
				entry={moveTarget}
				loading={
					moveTarget !== null && actionLoading === `${moveTarget.path}:move`
				}
				onCancel={() => setMoveTarget(null)}
				onOk={(destination) => void submitMove(destination)}
			/>
			<MkdirModal
				key={String(mkdirOpen)}
				open={mkdirOpen}
				loading={actionLoading === `${path}:mkdir`}
				onCancel={() => setMkdirOpen(false)}
				onOk={(name) => void submitMkdir(name)}
			/>
			<BatchRenameModal
				storageId={storageId}
				path={path}
				open={batchRenameOpen}
				onClose={() => setBatchRenameOpen(false)}
				onDone={() => {
					setBatchRenameOpen(false);
					void browse(path);
				}}
			/>
			<TmdbModal
				storageId={storageId}
				path={path}
				open={tmdbOpen}
				onClose={() => setTmdbOpen(false)}
				onDone={() => {
					setTmdbOpen(false);
					void browse(path);
				}}
			/>
			{menu && (
				<ContextMenu
					x={menu.x}
					y={menu.y}
					items={[{ key: "copy", label: "复制STRM地址" }]}
					onSelect={(key) => {
						const { entry } = menu;
						setMenu(null);
						if (key === "copy") void copyURL(entry);
					}}
					onClose={() => setMenu(null)}
				/>
			)}
		</section>
	);
}
