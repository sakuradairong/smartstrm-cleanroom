import { useCallback, useEffect, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import {
	Alert,
	App,
	Button,
	Card,
	Collapse,
	Descriptions,
	Empty,
	Popconfirm,
	Space,
	Spin,
	Table,
	Tabs,
	Tooltip,
	Typography,
} from "antd";
import { QuestionCircleOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { getJSON, postJSON, putJSON } from "../api/client";
import type { ManagedConfig, StorageConfig } from "../api/types";
import type { TaskConfig } from "../api/types";
import { useLocation, useNavigate } from "react-router";
import StorageFormModal from "./config/StorageFormModal";
import TaskFormModal from "./config/TaskFormModal";
import JsonField, { validateJSON } from "../components/JsonField";
import PathMappingField, {
	recordFromRows,
	rowsFromRecord,
} from "../components/PathMappingField";
import type { MappingRow } from "../components/PathMappingField";
import SecretInput from "../components/SecretInput";
import "./ConfigPage.css";

/** A labelled row inside a settings group, with an optional explanation. */
function SettingRow({
	label,
	hint,
	htmlFor,
	children,
}: {
	label: string;
	hint?: string;
	htmlFor?: string;
	children: React.ReactNode;
}) {
	return (
		<div className="setting-row">
			<label className="setting-label" htmlFor={htmlFor}>
				{label}
				{hint ? (
					<Tooltip title={hint}>
						<QuestionCircleOutlined
							className="setting-hint-icon"
							aria-label={`${label}说明`}
						/>
					</Tooltip>
				) : null}
			</label>
			<div className="setting-control">{children}</div>
		</div>
	);
}

interface BootstrapConfig {
	listen: string;
	public_url: string;
	admin_username: string;
}

interface ManagedConfigResponse {
	managed: ManagedConfig;
	bootstrap: BootstrapConfig;
	restart_required: boolean;
	backup_available: boolean;
}

interface SavedConfigResponse {
	managed: ManagedConfig;
	restart_required: boolean;
}

interface TokenResponse {
	webhook_token: string;
	restart_required: boolean;
}

type ConfigTabKey = "basic" | "storages" | "tasks" | "advanced";

function configTabFromSearch(search: string): ConfigTabKey {
	const section = new URLSearchParams(search).get("section");
	return section === "storages" || section === "tasks" || section === "advanced"
		? section
		: "basic";
}

export default function ConfigPage() {
	const { message, modal } = App.useApp();
	const location = useLocation();
	const navigate = useNavigate();
	const [draft, setDraft] = useState<ManagedConfig | null>(null);
	const [bootstrap, setBootstrap] = useState<BootstrapConfig | null>(null);
	const [loading, setLoading] = useState(true);
	const [dirty, setDirty] = useState(false);
	const [saving, setSaving] = useState(false);
	const [restartNeeded, setRestartNeeded] = useState(false);
	const [backupAvailable, setBackupAvailable] = useState(false);
	const [storageIndex, setStorageIndex] = useState<number | null>(null);
	const [taskIndex, setTaskIndex] = useState<number | null>(null);
	const [activeTab, setActiveTab] = useState<ConfigTabKey>(() =>
		configTabFromSearch(location.search),
	);
	const addTaskRequested =
		new URLSearchParams(location.search).get("action") === "add-task";
	const [error, setError] = useState("");
	const [cloudDriveRows, setCloudDriveRows] = useState<MappingRow[]>([]);
	const [moviePilotRows, setMoviePilotRows] = useState<MappingRow[]>([]);
	const [globalPlugins, setGlobalPlugins] = useState("[]");

	const syncJSONFields = (managed: ManagedConfig) => {
		setCloudDriveRows(rowsFromRecord(managed.integrations.clouddrive2));
		setMoviePilotRows(rowsFromRecord(managed.integrations.moviepilot));
		setGlobalPlugins(JSON.stringify(managed.plugins ?? [], null, 2));
	};

	const load = useCallback(async () => {
		setLoading(true);
		try {
			const response = await getJSON<ManagedConfigResponse>(
				"/api/config/managed",
			);
			setDraft(response.managed);
			syncJSONFields(response.managed);
			setBootstrap(response.bootstrap);
			setBackupAvailable(response.backup_available);
			setDirty(false);
			setError("");
		} catch (loadError) {
			const text =
				loadError instanceof Error ? loadError.message : "设置加载失败";
			setError(text);
			void message.error(text);
		} finally {
			setLoading(false);
		}
	}, [message]);

	useEffect(() => {
		void load();
	}, [load]);

	useEffect(() => {
		setActiveTab(configTabFromSearch(location.search));
	}, [location.search]);

	useEffect(() => {
		const warnBeforeUnload = (event: BeforeUnloadEvent) => {
			if (!dirty) return;
			event.preventDefault();
			event.returnValue = "";
		};
		window.addEventListener("beforeunload", warnBeforeUnload);
		return () => window.removeEventListener("beforeunload", warnBeforeUnload);
	}, [dirty]);

	const saveStorage = (storage: StorageConfig, index: number) => {
		setDraft((current) => {
			if (!current) return current;
			const storages = [...current.storages];
			if (index < 0) storages.push(storage);
			else storages[index] = storage;
			return { ...current, storages };
		});
		setDirty(true);
		setStorageIndex(null);
		void message.info("存储已加入待应用修改，请点击“应用所有修改”使其生效");
	};

	const deleteStorage = (index: number) => {
		if (!draft) return;
		const storage = draft.storages[index];
		const used = draft.tasks.filter((task) => task.storage_id === storage.id);
		if (used.length) {
			modal.error({
				title: "无法删除存储",
				content: `请先删除或调整引用该存储的任务：${used.map((task) => task.id).join(", ")}`,
			});
			return;
		}
		setDraft({
			...draft,
			storages: draft.storages.filter((_, itemIndex) => itemIndex !== index),
		});
		setDirty(true);
	};

	const saveTask = (task: TaskConfig, index: number) => {
		setDraft((current) => {
			if (!current) return current;
			const tasks = [...current.tasks];
			if (index < 0) tasks.push(task);
			else tasks[index] = task;
			return { ...current, tasks };
		});
		setDirty(true);
		setTaskIndex(null);
		void message.info("任务已加入待应用修改，请点击“应用所有修改”使其生效");
	};

	const addTask = () => {
		if (!draft?.storages.length) {
			modal.warning({ title: "无法添加任务", content: "请先添加存储" });
			return;
		}
		setTaskIndex(-1);
	};

	useEffect(() => {
		if (!draft || !addTaskRequested) return;
		setActiveTab("tasks");
	}, [addTaskRequested, draft]);

	const changeTab = (key: string) => {
		const tab = key as ConfigTabKey;
		setActiveTab(tab);
		const search = new URLSearchParams(location.search);
		if (tab === "basic") search.delete("section");
		else search.set("section", tab);
		if (tab !== "tasks") search.delete("action");
		navigate(
			{
				pathname: location.pathname,
				search: search.toString() ? `?${search.toString()}` : "",
			},
			{ replace: true },
		);
	};

	const closeTaskModal = () => {
		setTaskIndex(null);
		const search = new URLSearchParams(location.search);
		if (search.get("action") !== "add-task") return;
		search.delete("action");
		navigate(
			{
				pathname: location.pathname,
				search: search.toString() ? `?${search.toString()}` : "",
			},
			{ replace: true },
		);
	};

	const deleteTask = (index: number) => {
		if (!draft) return;
		setDraft({
			...draft,
			tasks: draft.tasks.filter((_, taskItemIndex) => taskItemIndex !== index),
		});
		setDirty(true);
	};

	const resetToken = async () => {
		try {
			const response = await postJSON<TokenResponse>(
				"/api/config/webhook-token",
			);
			setRestartNeeded(response.restart_required);
			modal.info({
				title: "新 Webhook Token",
				content: (
					<Space direction="vertical">
						<Typography.Text>新 Token 仅显示一次，请立即保存。</Typography.Text>
						<Typography.Text code copyable>
							{response.webhook_token}
						</Typography.Text>
					</Space>
				),
			});
		} catch (tokenError) {
			void message.error(
				tokenError instanceof Error ? tokenError.message : "重置失败",
			);
		}
	};

	const save = async () => {
		if (!draft) return;
		if (validateJSON(globalPlugins)) {
			setError("请先修正高级能力中的 JSON 格式错误");
			return;
		}
		let plugins: unknown;
		try {
			plugins = JSON.parse(globalPlugins) as unknown;
		} catch {
			setError("请先修正高级能力中的 JSON 格式错误");
			return;
		}
		if (!Array.isArray(plugins)) {
			setError("全局插件必须是 JSON 数组");
			return;
		}
		const payload: ManagedConfig = {
			...draft,
			integrations: {
				clouddrive2: recordFromRows(cloudDriveRows),
				moviepilot: recordFromRows(moviePilotRows),
			},
			plugins: plugins as ManagedConfig["plugins"],
		};
		setSaving(true);
		setError("");
		try {
			const response = await putJSON<SavedConfigResponse>(
				"/api/config/managed",
				payload,
			);
			setDraft(response.managed);
			syncJSONFields(response.managed);
			setDirty(false);
			setRestartNeeded(response.restart_required);
			void message.success(
				response.restart_required
					? "配置已保存；动态设置已生效，启动级设置需重启"
					: "配置已保存并动态生效，无需重启",
			);
		} catch (saveError) {
			const text = saveError instanceof Error ? saveError.message : "保存失败";
			setError(text);
			void message.error(text);
		} finally {
			setSaving(false);
		}
	};

	const restore = async () => {
		try {
			const response = await postJSON<{ restart_required: boolean }>(
				"/api/config/restore",
			);
			await load();
			setRestartNeeded(response.restart_required);
			void message.success(
				response.restart_required
					? "备份已恢复；启动级设置需重启生效"
					: "备份已恢复并已动态生效",
			);
		} catch (restoreError) {
			void message.error(
				restoreError instanceof Error ? restoreError.message : "恢复失败",
			);
		}
	};

	const columns: ColumnsType<StorageConfig> = [
		{ title: "标识", dataIndex: "id" },
		{ title: "类型", dataIndex: "type" },
		{
			title: "根路径 / 服务地址",
			render: (_, storage) => (
				<Space direction="vertical" size={0}>
					{storage.root ? <span>{storage.root}</span> : null}
					{storage.endpoint ? (
						<Typography.Text type="secondary">
							{storage.endpoint}
						</Typography.Text>
					) : null}
					{storage.type === "123_open" ? (
						<Typography.Text type="secondary">
							挂载 File ID：{storage.root_file_id || "0"}
						</Typography.Text>
					) : null}
				</Space>
			),
		},
		{
			title: "操作",
			render: (_, storage, index) => (
				<Space>
					<Button type="link" onClick={() => setStorageIndex(index)}>
						编辑
					</Button>
					<Popconfirm
						title={`从配置中删除存储 ${storage.id}？不会删除远端文件。`}
						okText="删除"
						cancelText="取消"
						okButtonProps={{ danger: true }}
						onConfirm={() => deleteStorage(index)}
					>
						<Button type="link" danger>
							删除
						</Button>
					</Popconfirm>
				</Space>
			),
		},
	];

	const taskColumns: ColumnsType<TaskConfig> = [
		{ title: "标识", dataIndex: "id" },
		{ title: "名称", dataIndex: "name" },
		{ title: "存储", dataIndex: "storage_id" },
		{ title: "源路径", dataIndex: "source" },
		{
			title: "调度",
			dataIndex: "schedule",
			render: (value) => value || "手动",
		},
		{
			title: "操作",
			render: (_, task, index) => (
				<Space>
					<Button type="link" onClick={() => setTaskIndex(index)}>
						编辑
					</Button>
					<Popconfirm
						title={`从配置中删除任务 ${task.id}？现有 STRM 文件不会自动删除。`}
						okText="删除"
						cancelText="取消"
						okButtonProps={{ danger: true }}
						onConfirm={() => deleteTask(index)}
					>
						<Button type="link" danger>
							删除
						</Button>
					</Popconfirm>
				</Space>
			),
		},
	];

	if (loading && (!draft || !bootstrap)) {
		return (
			<Spin tip="加载配置中">
				<div style={{ minHeight: 160 }} />
			</Spin>
		);
	}

	if (!draft || !bootstrap) {
		return (
			<Card>
				<Empty
					description={
						<Space direction="vertical" size={4}>
							<Typography.Text strong>配置加载失败</Typography.Text>
							<Typography.Text type="secondary">
								{error || "无法连接到 SmartStrm 服务，请检查网络后重试。"}
							</Typography.Text>
						</Space>
					}
				>
					<Space>
						<Button
							type="primary"
							loading={loading}
							onClick={() => void load()}
						>
							重新加载
						</Button>
						<Button onClick={() => navigate("/")}>返回仪表盘</Button>
					</Space>
				</Empty>
			</Card>
		);
	}

	return (
		<Space
			className="config-page"
			direction="vertical"
			size="large"
			style={{ width: "100%" }}
		>
			<header className="management-heading">
				<div>
					<h2>配置</h2>
					<Typography.Text type="secondary">
						管理媒体存储、STRM 任务与系统运行参数
					</Typography.Text>
				</div>
			</header>
			<Alert
				type="info"
				showIcon
				message="首次使用只需两步：先添加存储，再创建任务"
				description="存储告诉 SmartStrm 去哪里读取媒体；任务决定读取哪个目录、把 STRM 文件保存到哪里。修改会先成为草稿，最后点击“应用所有修改”才会生效。"
			/>
			{error ? <Alert type="error" showIcon message={error} /> : null}
			<Tabs
				className="config-tabs"
				activeKey={activeTab}
				onChange={changeTab}
				items={[
					{
						key: "basic",
						label: "基础配置",
						children: (
							<Card
								id="config-basic"
								title="服务访问设置（只读）"
								extra={
									<Popconfirm
										title="生成新 Webhook Token？当前 Token 在重启后失效。"
										okText="重置"
										cancelText="取消"
										okButtonProps={{ danger: true }}
										onConfirm={() => void resetToken()}
									>
										<Button type="text" danger>
											重置 Webhook Token
										</Button>
									</Popconfirm>
								}
							>
								<Descriptions column={{ xs: 1, sm: 2, lg: 4 }}>
									<Descriptions.Item label="监听地址">
										<Typography.Text code>{bootstrap.listen}</Typography.Text>
									</Descriptions.Item>
									<Descriptions.Item label="公开地址">
										<Typography.Text code>
											{bootstrap.public_url}
										</Typography.Text>
									</Descriptions.Item>
									<Descriptions.Item label="管理员">
										<Typography.Text code>
											{bootstrap.admin_username || "（未启用）"}
										</Typography.Text>
									</Descriptions.Item>
									<Descriptions.Item label="Webhook Token">
										<Typography.Text code>已在基础文件中配置</Typography.Text>
									</Descriptions.Item>
								</Descriptions>
							</Card>
						),
					},
					{
						key: "storages",
						label: "存储",
						children: (
							<Card
								id="config-storages"
								title="第 1 步：添加媒体存储"
								extra={
									<Button type="primary" onClick={() => setStorageIndex(-1)}>
										添加存储
									</Button>
								}
							>
								<Table
									rowKey="id"
									columns={columns}
									dataSource={draft.storages}
									pagination={false}
									locale={{
										emptyText: (
											<Empty description="尚未添加存储，请先添加媒体文件所在的位置">
												<Button
													type="primary"
													onClick={() => setStorageIndex(-1)}
												>
													添加第一个存储
												</Button>
											</Empty>
										),
									}}
									scroll={{ x: 700 }}
								/>
							</Card>
						),
					},
					{
						key: "tasks",
						label: "生成任务",
						children: (
							<Card
								id="config-tasks"
								title="第 2 步：创建 STRM 任务"
								extra={
									<Button
										type="primary"
										disabled={!draft.storages.length}
										onClick={addTask}
									>
										添加任务
									</Button>
								}
							>
								<Table
									rowKey="id"
									columns={taskColumns}
									dataSource={draft.tasks}
									pagination={false}
									locale={{
										emptyText: (
											<Empty
												description={
													draft.storages.length
														? "尚未添加任务，请创建第一个 STRM 任务"
														: "添加存储后即可创建任务"
												}
											>
												{draft.storages.length ? (
													<Button type="primary" onClick={addTask}>
														创建第一个任务
													</Button>
												) : null}
											</Empty>
										),
									}}
									scroll={{ x: 900 }}
								/>
							</Card>
						),
					},
					{
						key: "advanced",
						label: "高级能力",
						children: (
							<Space
								id="config-advanced"
								direction="vertical"
								size="middle"
								style={{ width: "100%" }}
							>
								<Alert
									type="info"
									showIcon
									message="这些设置都有默认值，不确定时保持原样即可"
									description="只有开启对应功能时才需要修改：想用影视改名就填 TMDB，想抽取封面就填 FFmpeg，与 CloudDrive2 / MoviePilot 联动才需路径映射。保存后设置立即生效，只有修改媒体代理的监听端口需要重启。"
								/>

								<Card
									size="small"
									title="影视元数据（TMDB）"
									extra={
										<Typography.Text type="secondary">
											用于影视识别与智能改名
										</Typography.Text>
									}
								>
									<Typography.Paragraph
										type="secondary"
										style={{ marginTop: 0 }}
									>
										不使用“影视识别”功能可以全部留空。API Key 在 TMDB
										官网免费申请。
									</Typography.Paragraph>
									<SettingRow
										label="TMDB API Key"
										htmlFor="tmdb-api-key"
										hint="在 themoviedb.org 账号设置→API 中获取；留空则影视识别功能不可用。"
									>
										<SecretInput
											id="tmdb-api-key"
											value={draft.tmdb.api_key}
											onChange={(value) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													tmdb: { ...current.tmdb, api_key: value },
												}))
											}
										/>
									</SettingRow>
									<SettingRow
										label="TMDB 语言"
										htmlFor="tmdb-language"
										hint="影片标题与简介的语言，例如 zh-CN、en-US。"
									>
										<input
											className="config-control"
											id="tmdb-language"
											placeholder="zh-CN"
											value={draft.tmdb.language}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													tmdb: {
														...current.tmdb,
														language: event.target.value,
													},
												}))
											}
										/>
									</SettingRow>
									<SettingRow
										label="缓存分钟数"
										htmlFor="tmdb-cache"
										hint="查询结果缓存时长，0 表示使用默认的 30 分钟。"
									>
										<input
											className="config-control"
											id="tmdb-cache"
											type="number"
											inputMode="numeric"
											min={0}
											value={draft.tmdb.cache_minutes ?? 0}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													tmdb: {
														...current.tmdb,
														cache_minutes: Number(event.target.value),
													},
												}))
											}
										/>
									</SettingRow>
									<Collapse
										ghost
										items={[
											{
												key: "tmdb-network",
												label: "网络高级选项（国内网络不通时才需要）",
												children: (
													<>
														<SettingRow
															label="TMDB API 地址"
															htmlFor="tmdb-base-url"
															hint="反向代理地址；留空使用官方 api.themoviedb.org。"
														>
															<input
																className="config-control"
																id="tmdb-base-url"
																placeholder="留空使用官方地址"
																value={draft.tmdb.base_url ?? ""}
																onChange={(event) =>
																	updateDraft(
																		setDraft,
																		setDirty,
																		(current) => ({
																			...current,
																			tmdb: {
																				...current.tmdb,
																				base_url: event.target.value,
																			},
																		}),
																	)
																}
															/>
														</SettingRow>
														<SettingRow
															label="图片地址"
															htmlFor="tmdb-image-url"
															hint="海报图片的反向代理地址；留空使用官方 image.tmdb.org。"
														>
															<input
																className="config-control"
																id="tmdb-image-url"
																placeholder="留空使用官方地址"
																value={draft.tmdb.image_base_url ?? ""}
																onChange={(event) =>
																	updateDraft(
																		setDraft,
																		setDirty,
																		(current) => ({
																			...current,
																			tmdb: {
																				...current.tmdb,
																				image_base_url: event.target.value,
																			},
																		}),
																	)
																}
															/>
														</SettingRow>
														<SettingRow
															label="代理 URL"
															htmlFor="tmdb-proxy-url"
															hint="通过 HTTP/SOCKS 代理访问 TMDB，例如 http://127.0.0.1:7890。"
														>
															<input
																className="config-control"
																id="tmdb-proxy-url"
																placeholder="例如 http://127.0.0.1:7890"
																value={draft.tmdb.proxy_url ?? ""}
																onChange={(event) =>
																	updateDraft(
																		setDraft,
																		setDirty,
																		(current) => ({
																			...current,
																			tmdb: {
																				...current.tmdb,
																				proxy_url: event.target.value,
																			},
																		}),
																	)
																}
															/>
														</SettingRow>
													</>
												),
											},
										]}
									/>
								</Card>

								<Card
									size="small"
									title="运行日志"
									extra={
										<Typography.Text type="secondary">
											控制日志保存位置与数量
										</Typography.Text>
									}
								>
									<SettingRow
										label="历史文件路径"
										htmlFor="history-path"
										hint="服务端绝对路径；留空则保存在配置文件旁边的 history.jsonl。"
									>
										<input
											className="config-control"
											id="history-path"
											placeholder="留空使用默认位置"
											value={draft.history.path}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													history: {
														...current.history,
														path: event.target.value,
													},
												}))
											}
										/>
									</SettingRow>
									<SettingRow
										label="历史最大条数"
										htmlFor="history-max"
										hint="超出条数后自动丢弃最旧记录；0 表示使用默认的 2000 条。"
									>
										<input
											className="config-control"
											id="history-max"
											type="number"
											inputMode="numeric"
											min={0}
											max={100000}
											value={draft.history.max_entries}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													history: {
														...current.history,
														max_entries: Number(event.target.value),
													},
												}))
											}
										/>
									</SettingRow>
								</Card>

								<Card
									size="small"
									title="媒体工具"
									extra={
										<Typography.Text type="secondary">
											用于从视频抽取封面图
										</Typography.Text>
									}
								>
									<Typography.Paragraph
										type="secondary"
										style={{ marginTop: 0 }}
									>
										官方镜像已内置 ffmpeg，通常不需要修改。
									</Typography.Paragraph>
									<SettingRow
										label="FFmpeg 路径"
										htmlFor="ffmpeg-path"
										hint="可执行文件路径或命令名；留空默认使用 ffmpeg。"
									>
										<input
											className="config-control"
											id="ffmpeg-path"
											placeholder="ffmpeg"
											value={draft.media_tools.ffmpeg}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													media_tools: {
														...current.media_tools,
														ffmpeg: event.target.value,
													},
												}))
											}
										/>
									</SettingRow>
									<SettingRow
										label="媒体工具超时（秒）"
										htmlFor="media-timeout"
										hint="单个封面抽取的最长耗时；0 表示使用默认的 60 秒。"
									>
										<input
											className="config-control"
											id="media-timeout"
											type="number"
											inputMode="numeric"
											min={0}
											value={draft.media_tools.timeout_seconds}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													media_tools: {
														...current.media_tools,
														timeout_seconds: Number(event.target.value),
													},
												}))
											}
										/>
									</SettingRow>
								</Card>

								<Card
									size="small"
									title="媒体代理"
									extra={
										<Typography.Text type="secondary">
											让 Emby / Jellyfin 直连原画播放
										</Typography.Text>
									}
								>
									<Typography.Paragraph
										type="secondary"
										style={{ marginTop: 0 }}
									>
										开启后，SmartStrm
										会代理你的媒体服务器，把播放地址改写为网盘直链。保存后立即生效，无需重启。
										<Typography.Text strong>
											只有修改“代理监听端口”需要重启容器，因为端口映射在容器启动时就固定了。
										</Typography.Text>
									</Typography.Paragraph>
									<SettingRow
										label="启用媒体代理"
										htmlFor="media-proxy-enabled"
										hint="关闭时下方其余代理选项均不生效。"
									>
										<input
											id="media-proxy-enabled"
											type="checkbox"
											checked={draft.media_proxy.enabled}
											onChange={(event) =>
												updateDraft(setDraft, setDirty, (current) => ({
													...current,
													media_proxy: {
														...current.media_proxy,
														enabled: event.target.checked,
													},
												}))
											}
										/>
									</SettingRow>
									{draft.media_proxy.enabled ? (
										<>
											<SettingRow
												label="媒体代理监听"
												htmlFor="media-proxy-listen"
												hint="SmartStrm 代理监听的端口，例如 :8097；需在容器中映射该端口。修改端口后需重启容器才生效。"
											>
												<input
													className="config-control"
													id="media-proxy-listen"
													placeholder=":8097"
													value={draft.media_proxy.listen}
													onChange={(event) =>
														updateDraft(setDraft, setDirty, (current) => ({
															...current,
															media_proxy: {
																...current.media_proxy,
																listen: event.target.value,
															},
														}))
													}
												/>
											</SettingRow>
											<SettingRow
												label="媒体代理上游"
												htmlFor="media-proxy-upstream"
												hint="你原有的 Emby / Jellyfin 地址，例如 http://emby:8096。"
											>
												<input
													className="config-control"
													id="media-proxy-upstream"
													placeholder="http://emby:8096"
													value={draft.media_proxy.upstream}
													onChange={(event) =>
														updateDraft(setDraft, setDirty, (current) => ({
															...current,
															media_proxy: {
																...current.media_proxy,
																upstream: event.target.value,
															},
														}))
													}
												/>
											</SettingRow>
											<SettingRow
												label="媒体服务器类型"
												htmlFor="media-proxy-type"
												hint="开启“改写 PlaybackInfo”时必须选择。"
											>
												<select
													className="config-control"
													id="media-proxy-type"
													value={draft.media_proxy.server_type}
													onChange={(event) =>
														updateDraft(setDraft, setDirty, (current) => ({
															...current,
															media_proxy: {
																...current.media_proxy,
																server_type: event.target.value,
															},
														}))
													}
												>
													<option value="">请选择</option>
													<option value="emby">Emby</option>
													<option value="jellyfin">Jellyfin</option>
												</select>
											</SettingRow>
											<SettingRow
												label="改写 PlaybackInfo"
												htmlFor="media-proxy-rewrite"
												hint="让播放器直连网盘直链，避免服务器中转；需先选择服务器类型。"
											>
												<input
													id="media-proxy-rewrite"
													type="checkbox"
													checked={draft.media_proxy.rewrite_playback_info}
													onChange={(event) =>
														updateDraft(setDraft, setDirty, (current) => ({
															...current,
															media_proxy: {
																...current.media_proxy,
																rewrite_playback_info: event.target.checked,
															},
														}))
													}
												/>
											</SettingRow>
										</>
									) : null}
								</Card>

								<Card
									size="small"
									title="外部联动路径映射"
									extra={
										<Typography.Text type="secondary">
											与 CloudDrive2 / MoviePilot 配合使用
										</Typography.Text>
									}
								>
									<Typography.Paragraph
										type="secondary"
										style={{ marginTop: 0 }}
									>
										外部工具通知 SmartStrm 时会带上它自己的路径，这里告诉
										SmartStrm 该路径对应哪个存储位置。不使用这两个工具可以留空。
									</Typography.Paragraph>
									<div className="setting-block">
										<label
											className="setting-label"
											htmlFor="clouddrive-mappings"
										>
											CloudDrive2 路径映射
										</label>
										<PathMappingField
											id="clouddrive-mappings"
											rows={cloudDriveRows}
											onChange={(rows) => {
												setCloudDriveRows(rows);
												setDirty(true);
											}}
											fromLabel="CloudDrive2 中的路径"
											toLabel="CloudDrive2 对应的存储位置"
											fromPlaceholder="/CloudNAS/115"
											toPlaceholder="存储标识/子目录"
											addLabel="添加 CloudDrive2 映射"
										/>
									</div>
									<div className="setting-block">
										<label
											className="setting-label"
											htmlFor="moviepilot-mappings"
										>
											MoviePilot 路径映射
										</label>
										<PathMappingField
											id="moviepilot-mappings"
											rows={moviePilotRows}
											onChange={(rows) => {
												setMoviePilotRows(rows);
												setDirty(true);
											}}
											fromLabel="MoviePilot 中的路径"
											toLabel="MoviePilot 对应的存储位置"
											fromPlaceholder="/media/movie"
											toPlaceholder="存储标识/子目录"
											addLabel="添加 MoviePilot 映射"
										/>
									</div>
								</Card>

								<Card
									size="small"
									title="全局插件"
									extra={
										<Typography.Text type="secondary">
											适用于所有任务
										</Typography.Text>
									}
								>
									<Typography.Paragraph
										type="secondary"
										style={{ marginTop: 0 }}
									>
										仅供熟悉插件配置的用户使用。不使用插件时保持{" "}
										<Typography.Text code>[]</Typography.Text>{" "}
										即可；单个任务的插件在任务编辑窗口里配置。
									</Typography.Paragraph>
									<div className="setting-block">
										<label className="setting-label" htmlFor="global-plugins">
											全局插件
										</label>
										<JsonField
											id="global-plugins"
											value={globalPlugins}
											onChange={(value) => {
												setGlobalPlugins(value);
												setDirty(true);
											}}
											schemaHint="JSON 数组"
										/>
									</div>
								</Card>
							</Space>
						),
					},
				]}
			/>

			<Card className="config-save-bar" data-dirty={dirty ? "true" : "false"}>
				<Space wrap>
					<Button
						type="primary"
						loading={saving}
						disabled={saving || !dirty}
						onClick={() => void save()}
					>
						应用所有修改
					</Button>
					<Popconfirm
						title="放弃当前所有尚未应用的修改？"
						description="此操作不会影响当前已生效的设置。"
						okText="放弃修改"
						cancelText="继续编辑"
						onConfirm={() => void load()}
					>
						<Button disabled={saving || !dirty}>放弃未应用修改</Button>
					</Popconfirm>
					<Popconfirm
						title="用上一次备份覆盖当前全部配置？"
						description="恢复后会立即应用动态设置；少数服务访问设置可能需要重启。"
						okText="恢复"
						cancelText="取消"
						okButtonProps={{ danger: true }}
						onConfirm={() => void restore()}
					>
						<Button disabled={saving || !backupAvailable}>
							恢复上一次备份
						</Button>
					</Popconfirm>
					<Typography.Text type="secondary" role="status" aria-live="polite">
						{restartNeeded
							? "常规设置已生效；服务访问设置等待重启"
							: dirty
								? "有待应用的修改，离开页面前请点击“应用所有修改”"
								: "所有设置已应用"}
					</Typography.Text>
				</Space>
			</Card>

			{restartNeeded ? (
				<Alert
					type="warning"
					showIcon
					message="常规设置已经生效；服务地址、管理员或媒体代理设置需要重启后完全生效。"
				/>
			) : null}

			<StorageFormModal
				open={storageIndex !== null}
				index={storageIndex ?? -1}
				storages={draft.storages}
				onSave={saveStorage}
				onClose={() => setStorageIndex(null)}
			/>
			<TaskFormModal
				open={taskIndex !== null || addTaskRequested}
				index={taskIndex ?? -1}
				tasks={draft.tasks}
				storages={draft.storages}
				onSave={saveTask}
				onClose={closeTaskModal}
			/>
		</Space>
	);
}

function updateDraft(
	setDraft: Dispatch<SetStateAction<ManagedConfig | null>>,
	setDirty: Dispatch<SetStateAction<boolean>>,
	update: (current: ManagedConfig) => ManagedConfig,
) {
	setDraft((current) => (current ? update(current) : current));
	setDirty(true);
}
