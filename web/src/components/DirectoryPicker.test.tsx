import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App as AntApp } from "antd";
import DirectoryPicker from "./DirectoryPicker";

function response(body: unknown, status = 200): Response {
	return {
		ok: status < 400,
		status,
		text: async () => JSON.stringify(body),
	} as Response;
}

afterEach(() => vi.unstubAllGlobals());

test("存储模式仅列出目录并可逐层进入", async () => {
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/storages/media/entries?path=%2F") {
				return response([
					{ path: "/电影", name: "电影", size: 0, is_dir: true, modified: "" },
					{
						path: "/readme.txt",
						name: "readme.txt",
						size: 10,
						is_dir: false,
						modified: "",
					},
				]);
			}
			if (url === "/api/storages/media/entries?path=%2F%E7%94%B5%E5%BD%B1") {
				return response([
					{
						path: "/电影/2024",
						name: "2024",
						size: 0,
						is_dir: true,
						modified: "",
					},
				]);
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	const onSelect = vi.fn();
	const user = userEvent.setup();
	render(
		<AntApp>
			<DirectoryPicker
				open
				mode="storage"
				title="选择源目录"
				storageID="media"
				value="/"
				onCancel={() => {}}
				onSelect={onSelect}
			/>
		</AntApp>,
	);

	expect(await screen.findByText("电影")).toBeInTheDocument();
	expect(screen.queryByText("readme.txt")).not.toBeInTheDocument();

	await user.click(screen.getByText("电影"));
	expect(await screen.findByText("2024")).toBeInTheDocument();
	await waitFor(() =>
		expect(screen.getByLabelText("当前目录")).toHaveValue("/电影"),
	);

	await user.click(screen.getByRole("button", { name: "选择当前目录" }));
	expect(onSelect).toHaveBeenCalledWith("/电影");
});

test("本地模式读取 local-dirs 并可返回上一级", async () => {
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/local-dirs?path=%2Fstrm") {
				return response({
					path: "/strm",
					parent: "/",
					directories: [{ name: "电影", path: "/strm/电影" }],
				});
			}
			if (url === "/api/local-dirs?path=%2F") {
				return response({
					path: "/",
					parent: "",
					directories: [{ name: "strm", path: "/strm" }],
				});
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	const user = userEvent.setup();
	render(
		<AntApp>
			<DirectoryPicker
				open
				mode="local"
				title="选择输出目录"
				value="/strm"
				onCancel={() => {}}
				onSelect={() => {}}
			/>
		</AntApp>,
	);

	expect(await screen.findByText("电影")).toBeInTheDocument();
	await user.click(screen.getByRole("button", { name: "上一级" }));
	await waitFor(() =>
		expect(screen.getByLabelText("当前目录")).toHaveValue("/"),
	);
	// 根目录没有父级，按钮应禁用
	expect(screen.getByRole("button", { name: "上一级" })).toBeDisabled();
});

test("新建目录调用接口并刷新列表", async () => {
	const posted: unknown[] = [];
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url === "/api/local-dirs" && init?.method === "POST") {
				posted.push(JSON.parse(String(init.body)));
				return response({ name: "剧集", path: "/strm/剧集" }, 201);
			}
			if (url === "/api/local-dirs?path=%2Fstrm") {
				return response({
					path: "/strm",
					parent: "/",
					directories: posted.length
						? [{ name: "剧集", path: "/strm/剧集" }]
						: [],
				});
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	const user = userEvent.setup();
	render(
		<AntApp>
			<DirectoryPicker
				open
				mode="local"
				title="选择输出目录"
				value="/strm"
				onCancel={() => {}}
				onSelect={() => {}}
			/>
		</AntApp>,
	);

	await user.click(await screen.findByRole("button", { name: /新建目录/ }));
	await user.type(screen.getByLabelText("新目录名称"), "剧集");
	await user.click(screen.getByRole("button", { name: "创建" }));

	await waitFor(() =>
		expect(posted).toEqual([{ path: "/strm", name: "剧集" }]),
	);
	expect(await screen.findByText("剧集")).toBeInTheDocument();
});

test("读取失败显示后端错误", async () => {
	vi.stubGlobal(
		"fetch",
		vi.fn(async () => response({ error: "directory cannot be read" }, 403)),
	);
	render(
		<AntApp>
			<DirectoryPicker
				open
				mode="local"
				title="选择输出目录"
				value="/strm"
				onCancel={() => {}}
				onSelect={() => {}}
			/>
		</AntApp>,
	);
	expect(
		await screen.findByText("directory cannot be read"),
	).toBeInTheDocument();
});
