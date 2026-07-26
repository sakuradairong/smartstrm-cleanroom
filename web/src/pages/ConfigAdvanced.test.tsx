import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App as AntApp } from "antd";
import { MemoryRouter } from "react-router";
import ConfigPage from "./ConfigPage";
import type { ManagedConfig } from "../api/types";

const managed: ManagedConfig = {
	storages: [],
	tasks: [],
	plugins: [],
	integrations: { clouddrive2: { "/cloud": "/media" }, moviepilot: {} },
	tmdb: {
		api_key: "********",
		language: "zh-CN",
		cache_minutes: 60,
		base_url: "",
		image_base_url: "",
		proxy_url: "",
	},
	history: { path: "/data/history.jsonl", max_entries: 1000 },
	media_tools: { ffmpeg: "ffmpeg", timeout_seconds: 30 },
	media_proxy: {
		enabled: false,
		listen: ":8097",
		upstream: "http://emby:8096",
		server_type: "emby",
		rewrite_playback_info: false,
	},
};

function renderPage() {
	return render(
		<MemoryRouter initialEntries={["/config?section=advanced"]}>
			<AntApp>
				<ConfigPage />
			</AntApp>
		</MemoryRouter>,
	);
}

function setupFetch(putStatus = 200, restartRequired = false) {
	let putBody: ManagedConfig | undefined;
	vi.stubGlobal(
		"fetch",
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url === "/api/config/managed" && !init?.method)
				return response({
					managed,
					bootstrap: { listen: ":8024", public_url: "", admin_username: "" },
					backup_available: false,
				});
			if (url === "/api/config/managed" && init?.method === "PUT") {
				putBody = JSON.parse(String(init.body));
				return putStatus === 200
					? response({ managed: putBody, restart_required: restartRequired })
					: response({ error: "invalid cron" }, 400);
			}
			throw new Error(`unexpected ${url}`);
		}),
	);
	return () => putBody;
}

function response(body: unknown, status = 200): Response {
	return {
		ok: status < 400,
		status,
		text: async () => JSON.stringify(body),
	} as Response;
}

afterEach(() => vi.unstubAllGlobals());

test("渲染全部高级能力字段", async () => {
	setupFetch();
	renderPage();
	for (const label of [
		"TMDB API Key",
		"TMDB 语言",
		"缓存分钟数",
		"历史文件路径",
		"历史最大条数",
		"FFmpeg 路径",
		"媒体工具超时（秒）",
		"启用媒体代理",
		"全局插件",
	]) {
		expect(await screen.findByLabelText(label)).toBeInTheDocument();
	}
	// 路径映射已从裸 JSON 文本框改为表格，以分组标题呈现
	for (const heading of ["CloudDrive2 路径映射", "MoviePilot 路径映射"]) {
		expect(screen.getByText(heading)).toBeInTheDocument();
	}
});

test("TMDB 网络选项收进折叠区，展开后可编辑", async () => {
	setupFetch();
	const user = userEvent.setup();
	renderPage();
	expect(await screen.findByLabelText("TMDB API Key")).toBeInTheDocument();
	expect(screen.queryByLabelText("TMDB API 地址")).not.toBeInTheDocument();
	await user.click(screen.getByText(/网络高级选项/));
	for (const label of ["TMDB API 地址", "图片地址", "代理 URL"]) {
		expect(await screen.findByLabelText(label)).toBeInTheDocument();
	}
});

test("路径映射表格：回填、新增与删除后提交为对象", async () => {
	const getPutBody = setupFetch();
	const user = userEvent.setup();
	renderPage();

	// 已有映射回填为表格行，不再是 JSON 文本
	expect(await screen.findByDisplayValue("/cloud")).toBeInTheDocument();
	expect(screen.getByDisplayValue("/media")).toBeInTheDocument();

	await user.click(
		screen.getByRole("button", { name: /添加 MoviePilot 映射/ }),
	);
	// A change event matches the final user-visible edit without forcing Ant
	// Design's whole table to rerender once per character on constrained CI.
	fireEvent.change(screen.getByLabelText("MoviePilot 中的路径 1"), {
		target: { value: "/mp/movie" },
	});
	fireEvent.change(screen.getByLabelText("MoviePilot 对应的存储位置 1"), {
		target: { value: "openlist/movie" },
	});
	await user.click(screen.getByRole("button", { name: /应用所有修改/ }));

	await waitFor(() =>
		expect(getPutBody()?.integrations).toEqual({
			clouddrive2: { "/cloud": "/media" },
			moviepilot: { "/mp/movie": "openlist/movie" },
		}),
	);
});

test("路径映射删除行后不再提交该项", async () => {
	const getPutBody = setupFetch();
	const user = userEvent.setup();
	renderPage();
	await user.click(
		await screen.findByRole("button", { name: "删除CloudDrive2 中的路径 1" }),
	);
	await user.click(screen.getByRole("button", { name: /应用所有修改/ }));
	await waitFor(() =>
		expect(getPutBody()?.integrations.clouddrive2).toEqual({}),
	);
});

test("媒体代理启用后显示代理字段", async () => {
	setupFetch();
	const user = userEvent.setup();
	renderPage();
	await user.click(await screen.findByLabelText("启用媒体代理"));
	for (const label of [
		"媒体代理监听",
		"媒体代理上游",
		"媒体服务器类型",
		"改写 PlaybackInfo",
	])
		expect(screen.getByLabelText(label)).toBeInTheDocument();
});

test("TMDB Key 保持脱敏值提交并显示动态生效状态", async () => {
	const getPutBody = setupFetch();
	renderPage();
	fireEvent.change(await screen.findByLabelText("TMDB 语言"), {
		target: { value: "en-US" },
	});
	fireEvent.click(screen.getByRole("button", { name: /应用所有修改/ }));
	await waitFor(() => expect(getPutBody()?.tmdb.api_key).toBe("********"));
	expect(await screen.findByText("所有设置已应用")).toBeInTheDocument();
});

test("启动级设置待重启时显示精确提示", async () => {
	setupFetch(200, true);
	renderPage();
	fireEvent.change(await screen.findByLabelText("TMDB 语言"), {
		target: { value: "en-US" },
	});
	fireEvent.click(screen.getByRole("button", { name: /应用所有修改/ }));
	await waitFor(() =>
		expect(
			screen
				.getAllByRole("alert")
				.some((alert) => alert.textContent?.includes("常规设置已经生效")),
		).toBe(true),
	);
	expect(
		screen
			.getAllByRole("alert")
			.some((alert) => alert.textContent?.includes("需要重启后完全生效")),
	).toBe(true);
});

test("全局插件 JSON 非法时阻止保存", async () => {
	const getPutBody = setupFetch();
	const user = userEvent.setup();
	renderPage();
	const field = await screen.findByLabelText("全局插件");
	fireEvent.change(field, { target: { value: "{bad" } });
	await user.click(screen.getByRole("button", { name: /应用所有修改/ }));
	expect(getPutBody()).toBeUndefined();
	expect(
		screen.getByText(/请先修正高级能力中的 JSON 格式错误/),
	).toBeInTheDocument();
});

test("保存失败显示后端错误且草稿保留", async () => {
	setupFetch(400);
	renderPage();
	const language = await screen.findByLabelText("TMDB 语言");
	fireEvent.change(language, { target: { value: "en-US" } });
	fireEvent.click(screen.getByRole("button", { name: /应用所有修改/ }));
	expect((await screen.findAllByText("invalid cron")).length).toBeGreaterThan(
		0,
	);
	expect(language).toHaveValue("en-US");
});
