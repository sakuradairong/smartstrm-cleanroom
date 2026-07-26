import {
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App as AntApp } from "antd";
import TaskFormModal, { csv, csvText } from "./TaskFormModal";
import type { StorageConfig, TaskConfig } from "../../api/types";

const storage: StorageConfig = {
	id: "local",
	type: "local",
	root: "/media",
	endpoint: "",
	username: "",
	password: "",
	token: "",
};
const task: TaskConfig = {
	id: "movies",
	name: "电影",
	storage_id: "local",
	source: "/movies",
	destination: "/strm/movies",
	schedule: "0 1 * * *",
	media_ext: [".mkv", ".mp4"],
	copy_ext: [".nfo"],
	min_size: 1,
	max_size: 99,
	keep_local: ["poster.jpg"],
	incremental: true,
	sync_delete: false,
	watch: true,
	dir_time_check: false,
	file_id_mode: true,
	plugins: [],
};

function renderModal(
	props: Partial<React.ComponentProps<typeof TaskFormModal>> = {},
) {
	const onSave = vi.fn();
	render(
		<AntApp>
			<TaskFormModal
				open
				index={-1}
				tasks={[]}
				storages={[storage]}
				onSave={onSave}
				onClose={() => {}}
				{...props}
			/>
		</AntApp>,
	);
	return onSave;
}

test("渲染基本字段并在高级设置中回填已有任务", async () => {
	const user = userEvent.setup();
	renderModal({ index: 0, tasks: [task] });
	for (const label of [
		"任务标识",
		"任务名称",
		"从哪个存储读取",
		"源目录",
		"STRM 输出目录",
		"运行方式",
	]) {
		expect(screen.getByLabelText(label)).toBeInTheDocument();
	}
	expect(screen.getByLabelText("任务标识")).toHaveValue("movies");
	await user.click(screen.getByText("高级设置（一般无需修改）"));
	expect(screen.getByLabelText("识别为媒体的文件类型")).toHaveValue(
		".mkv, .mp4",
	);
	expect(screen.getByLabelText("持续监听文件变化")).toBeChecked();
});

test("扩展名 csv 双向转换", () => {
	expect(csv(".mkv, .mp4")).toEqual([".mkv", ".mp4"]);
	expect(csv("")).toEqual([]);
	expect(csvText([".mkv", ".mp4"])).toBe(".mkv, .mp4");
});

test("勾选 sync_delete 出现二次确认，取消后回滚", async () => {
	const user = userEvent.setup();
	renderModal();
	await user.click(screen.getByText("高级设置（一般无需修改）"));
	await user.click(screen.getByLabelText("同步删除目标中多余文件（危险）"));
	const warning = (
		await screen.findByText(/同步删除可能永久删除目标文件/)
	).closest<HTMLElement>('[role="dialog"]')!;
	await user.click(within(warning).getByRole("button", { name: /取\s*消/ }));
	expect(
		screen.getByLabelText("同步删除目标中多余文件（危险）"),
	).not.toBeChecked();
});

test("确认后启用 sync_delete", async () => {
	const user = userEvent.setup();
	renderModal();
	await user.click(screen.getByText("高级设置（一般无需修改）"));
	await user.click(screen.getByLabelText("同步删除目标中多余文件（危险）"));
	const warning = (
		await screen.findByText(/同步删除可能永久删除目标文件/)
	).closest<HTMLElement>('[role="dialog"]')!;
	await user.click(within(warning).getByRole("button", { name: /确认启用/ }));
	await waitFor(() =>
		expect(
			screen.getByLabelText("同步删除目标中多余文件（危险）"),
		).toBeChecked(),
	);
});

test("插件 JSON 非法时阻止保存并提示", async () => {
	const user = userEvent.setup();
	const onSave = renderModal({ index: 0, tasks: [task] });
	await user.click(screen.getByText("高级设置（一般无需修改）"));
	const plugins = screen.getByLabelText("插件配置（JSON）");
	fireEvent.change(plugins, { target: { value: "{bad" } });
	await user.click(screen.getByRole("button", { name: /加入待应用修改/ }));
	expect(
		await screen.findByText(/插件 JSON 不是合法 JSON/),
	).toBeInTheDocument();
	expect(onSave).not.toHaveBeenCalled();
});

test("编辑已有任务时 ID 不可改", () => {
	renderModal({ index: 0, tasks: [task] });
	expect(screen.getByLabelText("任务标识")).toBeDisabled();
});

test("存储为空列表时提示先添加存储并关闭", async () => {
	const onClose = vi.fn();
	render(
		<AntApp>
			<TaskFormModal
				open
				index={-1}
				tasks={[]}
				storages={[]}
				onSave={() => {}}
				onClose={onClose}
			/>
		</AntApp>,
	);
	expect(await screen.findByText("请先添加存储")).toBeInTheDocument();
	expect(onClose).toHaveBeenCalled();
	expect(screen.queryByText("添加任务")).not.toBeInTheDocument();
});

test("合法表单保存为 TaskConfig", async () => {
	const user = userEvent.setup();
	const onSave = renderModal({ index: 0, tasks: [task] });
	await user.click(screen.getByRole("button", { name: /加入待应用修改/ }));
	await waitFor(() =>
		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({
				id: "movies",
				media_ext: [".mkv", ".mp4"],
				plugins: [],
			}),
			0,
		),
	);
});

test("浏览源目录：从存储中选择后写回表单", async () => {
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/storages/local/entries?path=%2Fmovies") {
				return {
					ok: true,
					status: 200,
					text: async () =>
						JSON.stringify([
							{
								path: "/movies/2024",
								name: "2024",
								size: 0,
								is_dir: true,
								modified: "",
							},
						]),
				} as Response;
			}
			if (url === "/api/storages/local/entries?path=%2Fmovies%2F2024") {
				return {
					ok: true,
					status: 200,
					text: async () => JSON.stringify([]),
				} as Response;
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	const user = userEvent.setup();
	renderModal({ index: 0, tasks: [task] });

	await user.click(screen.getByRole("button", { name: "浏览源目录" }));
	await user.click(await screen.findByText("2024"));
	await user.click(await screen.findByRole("button", { name: "选择当前目录" }));

	await waitFor(() =>
		expect(screen.getByLabelText("源目录")).toHaveValue("/movies/2024"),
	);
	vi.unstubAllGlobals();
});

test("浏览输出目录：走本地目录接口而非存储接口", async () => {
	const requested: string[] = [];
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			requested.push(url);
			if (url.startsWith("/api/local-dirs")) {
				return {
					ok: true,
					status: 200,
					text: async () =>
						JSON.stringify({
							path: "/strm/movies",
							parent: "/strm",
							directories: [],
						}),
				} as Response;
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	const user = userEvent.setup();
	renderModal({ index: 0, tasks: [task] });

	await user.click(screen.getByRole("button", { name: "浏览输出目录" }));
	await waitFor(() =>
		expect(requested.some((url) => url.startsWith("/api/local-dirs"))).toBe(
			true,
		),
	);
	expect(requested.some((url) => url.includes("/entries"))).toBe(false);
	vi.unstubAllGlobals();
});
