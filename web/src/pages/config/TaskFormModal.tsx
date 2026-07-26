import { useEffect, useState } from "react";
import {
	Alert,
	App,
	Button,
	Checkbox,
	Collapse,
	Form,
	Input,
	InputNumber,
	Modal,
	Select,
	Space,
} from "antd";
import { FolderOpenOutlined } from "@ant-design/icons";
import type { PluginConfig, StorageConfig, TaskConfig } from "../../api/types";
import DirectoryPicker from "../../components/DirectoryPicker";

export interface TaskFormModalProps {
	open: boolean;
	index: number;
	tasks: TaskConfig[];
	storages: StorageConfig[];
	onSave: (task: TaskConfig, index: number) => void;
	onClose: () => void;
}

interface TaskFields {
	id: string;
	name: string;
	storage_id: string;
	source: string;
	destination: string;
	schedule_mode: string;
	schedule: string;
	media_ext: string;
	copy_ext: string;
	min_size: number;
	max_size: number;
	keep_local: string;
	incremental: boolean;
	sync_delete: boolean;
	watch: boolean;
	dir_time_check: boolean;
	file_id_mode: boolean;
	plugins: string;
}

const scheduleValues: Record<string, string> = {
	manual: "",
	every6hours: "0 */6 * * *",
	daily: "0 2 * * *",
	weekly: "0 2 * * 0",
};

function scheduleMode(schedule: string): string {
	const match = Object.entries(scheduleValues).find(
		([, value]) => value === schedule,
	);
	return match?.[0] ?? "custom";
}

function nextTaskID(tasks: TaskConfig[]): string {
	let index = 1;
	while (tasks.some((task) => task.id === `task-${index}`)) index += 1;
	return `task-${index}`;
}

export function csv(value: string): string[] {
	return String(value || "")
		.split(",")
		.map((item) => item.trim())
		.filter(Boolean);
}

export function csvText(value: string[] | undefined): string {
	return (value ?? []).join(", ");
}

const newTask = (storageID: string, tasks: TaskConfig[]): TaskFields => ({
	id: nextTaskID(tasks),
	name: "",
	storage_id: storageID,
	source: "/",
	destination: "/strm/",
	schedule_mode: "manual",
	schedule: "",
	media_ext: ".mkv, .mp4, .iso",
	copy_ext: ".jpg, .png, .nfo, .srt",
	min_size: 0,
	max_size: 0,
	keep_local: "",
	incremental: true,
	sync_delete: false,
	watch: false,
	dir_time_check: false,
	file_id_mode: false,
	plugins: "[]",
});

function fieldsFromTask(task: TaskConfig): TaskFields {
	return {
		...task,
		schedule_mode: scheduleMode(task.schedule),
		media_ext: csvText(task.media_ext),
		copy_ext: csvText(task.copy_ext),
		keep_local: csvText(task.keep_local),
		plugins: JSON.stringify(task.plugins ?? [], null, 2),
	};
}

export default function TaskFormModal({
	open,
	index,
	tasks,
	storages,
	onSave,
	onClose,
}: TaskFormModalProps) {
	const { modal } = App.useApp();
	const [form] = Form.useForm<TaskFields>();
	const [pluginError, setPluginError] = useState("");
	const [picker, setPicker] = useState<"source" | "destination" | null>(null);
	const previous = index >= 0 ? tasks[index] : undefined;
	const selectedScheduleMode = Form.useWatch("schedule_mode", form) ?? "manual";
	const selectedStorage =
		Form.useWatch("storage_id", form) ?? storages[0]?.id ?? "";
	const sourceValue = Form.useWatch("source", form) ?? "/";
	const destinationValue = Form.useWatch("destination", form) ?? "/strm/";

	useEffect(() => {
		if (!open) return;
		if (!storages.length) {
			modal.warning({ title: "无法添加任务", content: "请先添加存储" });
			onClose();
			return;
		}
		form.setFieldsValue(
			previous ? fieldsFromTask(previous) : newTask(storages[0].id, tasks),
		);
		setPluginError("");
	}, [form, modal, onClose, open, previous, storages]);

	const confirmSyncDelete = (checked: boolean) => {
		if (!checked) {
			form.setFieldValue("sync_delete", false);
			return;
		}
		form.setFieldValue("sync_delete", false);
		modal.confirm({
			title: "确认启用同步删除？",
			content:
				"同步删除可能永久删除目标文件，包括目标目录中源端已不存在的文件。确认在该任务中启用？",
			okText: "确认启用",
			okButtonProps: { danger: true },
			cancelText: "取消",
			onOk: () => form.setFieldValue("sync_delete", true),
		});
	};

	const save = (values: TaskFields) => {
		let plugins: PluginConfig[];
		try {
			const parsed: unknown = JSON.parse(values.plugins || "[]");
			if (!Array.isArray(parsed)) throw new Error("必须是 JSON 数组");
			plugins = parsed as PluginConfig[];
			setPluginError("");
		} catch (error) {
			const detail = error instanceof Error ? error.message : "格式错误";
			setPluginError(`插件 JSON 不是合法 JSON：${detail}`);
			return;
		}

		onSave(
			{
				id: values.id.trim(),
				name: values.name.trim(),
				storage_id: values.storage_id,
				source: values.source.trim(),
				destination: values.destination.trim(),
				schedule:
					values.schedule_mode === "custom"
						? values.schedule.trim()
						: (scheduleValues[values.schedule_mode] ?? ""),
				media_ext: csv(values.media_ext),
				copy_ext: csv(values.copy_ext),
				min_size: Number(values.min_size || 0),
				max_size: Number(values.max_size || 0),
				keep_local: csv(values.keep_local),
				incremental: Boolean(values.incremental),
				sync_delete: Boolean(values.sync_delete),
				watch: Boolean(values.watch),
				dir_time_check: Boolean(values.dir_time_check),
				file_id_mode: Boolean(values.file_id_mode),
				plugins,
			},
			index,
		);
	};

	const pathRule = { pattern: /^\//, message: "路径必须以 / 开头" };
	const destinationValidator = (_: unknown, value: string) =>
		value?.trim() === "/"
			? Promise.reject(
					new Error("输出目录不能是根目录 /，请使用例如 /strm/电影"),
				)
			: Promise.resolve();
	const scheduleValidator = (_: unknown, value: string) => {
		if (!value?.trim() || value.trim().split(/\s+/).length === 5)
			return Promise.resolve();
		return Promise.reject(
			new Error("调度必须是五段 cron 表达式，留空表示手动运行"),
		);
	};

	return (
		<Modal
			title={index >= 0 ? "编辑任务" : "添加任务"}
			open={open && storages.length > 0}
			width={760}
			okText="加入待应用修改"
			cancelText="取消"
			onOk={() => form.submit()}
			onCancel={onClose}
			destroyOnHidden
		>
			<Alert
				type="info"
				showIcon
				message="只需完成基本设置即可创建任务"
				description="任务会读取源目录中的媒体文件，并在目标目录生成供媒体服务器使用的 STRM 文件。高级设置可保持默认。"
				style={{ marginBottom: 16 }}
			/>
			<Form form={form} layout="vertical" onFinish={save}>
				<Space align="start" wrap style={{ width: "100%" }}>
					<Form.Item
						label="任务标识"
						name="id"
						extra={index >= 0 ? "创建后不可修改" : "已自动生成，一般无需修改"}
						rules={[
							{ required: true, message: "请输入任务标识" },
							{
								pattern: /^[A-Za-z0-9][A-Za-z0-9._-]*$/,
								message: "只能使用字母、数字、点、短横线和下划线",
							},
							{
								validator: (_, value: string) =>
									tasks.some(
										(task, taskIndex) =>
											taskIndex !== index && task.id === value?.trim(),
									)
										? Promise.reject(new Error("任务标识已存在，请换一个"))
										: Promise.resolve(),
							},
						]}
					>
						<Input disabled={index >= 0} />
					</Form.Item>
					<Form.Item
						label="任务名称"
						name="name"
						extra="例如：电影、电视剧"
						rules={[{ required: true, message: "请输入便于识别的任务名称" }]}
					>
						<Input placeholder="例如：电影" />
					</Form.Item>
					<Form.Item
						label="从哪个存储读取"
						name="storage_id"
						rules={[{ required: true, message: "请选择存储" }]}
					>
						<Select
							style={{ minWidth: 200 }}
							options={storages.map((storage) => ({
								value: storage.id,
								label: storage.id,
							}))}
						/>
					</Form.Item>
				</Space>
				<Form.Item
					label="源目录"
					name="source"
					extra="要扫描的媒体目录。可点“浏览”从存储中选择，填写 / 表示整个存储。"
					rules={[{ required: true, message: "请输入源目录" }, pathRule]}
				>
					<Input
						placeholder="例如 /电影"
						addonAfter={
							<Button
								type="text"
								size="small"
								aria-label="浏览源目录"
								icon={<FolderOpenOutlined />}
								onClick={() => setPicker("source")}
							>
								浏览
							</Button>
						}
					/>
				</Form.Item>
				<Form.Item
					label="STRM 输出目录"
					name="destination"
					extra="生成的 STRM 文件保存位置（服务端路径）。可点“浏览”选择或新建目录，建议使用 /strm/ 开头的独立目录。"
					rules={[
						{ required: true, message: "请输入 STRM 输出目录" },
						pathRule,
						{ validator: destinationValidator },
					]}
				>
					<Input
						placeholder="例如 /strm/电影"
						addonAfter={
							<Button
								type="text"
								size="small"
								aria-label="浏览输出目录"
								icon={<FolderOpenOutlined />}
								onClick={() => setPicker("destination")}
							>
								浏览
							</Button>
						}
					/>
				</Form.Item>
				<Form.Item label="运行方式" name="schedule_mode">
					<Select
						options={[
							{ value: "manual", label: "仅手动运行（推荐先选择）" },
							{ value: "every6hours", label: "每 6 小时自动运行" },
							{ value: "daily", label: "每天凌晨 2 点运行" },
							{ value: "weekly", label: "每周日凌晨 2 点运行" },
							{ value: "custom", label: "自定义时间（高级）" },
						]}
					/>
				</Form.Item>
				{selectedScheduleMode === "custom" ? (
					<Form.Item
						label="自定义运行时间"
						name="schedule"
						rules={[
							{ required: true, message: "请输入自定义运行时间" },
							{ validator: scheduleValidator },
						]}
						extra="使用五段 Cron 表达式，例如 0 3 * * * 表示每天凌晨 3 点。"
					>
						<Input placeholder="0 3 * * *" />
					</Form.Item>
				) : null}
				<Collapse
					ghost
					items={[
						{
							key: "advanced",
							label: "高级设置（一般无需修改）",
							forceRender: true,
							children: (
								<>
									<Form.Item
										label="识别为媒体的文件类型"
										name="media_ext"
										extra="多个类型用英文逗号分隔。"
									>
										<Input placeholder=".mkv, .mp4, .iso" />
									</Form.Item>
									<Form.Item
										label="一起复制的附属文件类型"
										name="copy_ext"
										extra="例如海报、字幕和影片信息文件。"
									>
										<Input placeholder=".jpg, .png, .nfo, .srt" />
									</Form.Item>
									<Space align="start" wrap>
										<Form.Item label="最小文件大小（字节）" name="min_size">
											<InputNumber min={0} />
										</Form.Item>
										<Form.Item
											label="最大文件大小（字节）"
											name="max_size"
											extra="0 表示不限制"
										>
											<InputNumber min={0} />
										</Form.Item>
									</Space>
									<Form.Item
										label="保留在本地的文件名"
										name="keep_local"
										extra="多个名称用英文逗号分隔；不确定请留空。"
									>
										<Input placeholder="例如 poster.jpg" />
									</Form.Item>
									<Space wrap>
										<Form.Item name="incremental" valuePropName="checked">
											<Checkbox>只处理新增或变化的文件</Checkbox>
										</Form.Item>
										<Form.Item name="sync_delete" valuePropName="checked">
											<Checkbox
												onChange={(event) =>
													confirmSyncDelete(event.target.checked)
												}
											>
												同步删除目标中多余文件（危险）
											</Checkbox>
										</Form.Item>
										<Form.Item name="watch" valuePropName="checked">
											<Checkbox>持续监听文件变化</Checkbox>
										</Form.Item>
										<Form.Item name="dir_time_check" valuePropName="checked">
											<Checkbox>检查目录修改时间</Checkbox>
										</Form.Item>
										<Form.Item name="file_id_mode" valuePropName="checked">
											<Checkbox>使用文件 ID 生成地址</Checkbox>
										</Form.Item>
									</Space>
									<Form.Item
										label="插件配置（JSON）"
										name="plugins"
										extra="仅供熟悉插件配置的用户使用；不使用插件时保持 []。"
										validateStatus={pluginError ? "error" : undefined}
										help={pluginError || undefined}
									>
										<Input.TextArea
											rows={5}
											onChange={() => setPluginError("")}
										/>
									</Form.Item>
								</>
							),
						},
					]}
				/>
			</Form>
			<DirectoryPicker
				key={picker ?? "closed"}
				open={picker !== null}
				mode={picker === "destination" ? "local" : "storage"}
				title={picker === "destination" ? "选择 STRM 输出目录" : "选择源目录"}
				storageID={selectedStorage}
				value={picker === "destination" ? destinationValue : sourceValue}
				onCancel={() => setPicker(null)}
				onSelect={(path) => {
					form.setFieldValue(
						picker === "destination" ? "destination" : "source",
						path,
					);
					void form.validateFields([
						picker === "destination" ? "destination" : "source",
					]);
					setPicker(null);
				}}
			/>
		</Modal>
	);
}
