import { useState } from "react";
import { Alert, Button, Input, Space, Table, Tag, Typography } from "antd";
import type { TableProps } from "antd";
import type { HistoryEvent } from "../api/types";
import { formatTimestamp } from "../format";
import { useHistoryStream } from "../hooks/useHistoryStream";
import { useSearchParams } from "react-router";

const EVENT_LABELS: Record<string, string> = {
	queued: "已排队",
	started: "已开始",
	completed: "已完成",
	failed: "失败",
	stopped: "已停止",
};

// 与旧 UI 运行时（renderIndexHTML 注入后的 renderHistory）一致：
// 有错误显示 error + 失败计数，否则显示各项计数（含失败）
function resultText(event: HistoryEvent): string {
	if (event.error) return `${event.error} / 失败 ${event.failed || 0}`;
	return `生成 ${event.created || 0} / 复制 ${event.copied || 0} / 删除 ${event.removed || 0} / 跳过 ${event.skipped || 0} / 失败 ${event.failed || 0}`;
}

export default function HistoryPage() {
	const [searchParams, setSearchParams] = useSearchParams();
	const initialTaskID = searchParams.get("task_id") ?? "";
	const [keyword, setKeyword] = useState(initialTaskID);
	const [filter, setFilter] = useState(initialTaskID);
	const [retryKey, setRetryKey] = useState(0);
	const { events, connected, loading, error } = useHistoryStream(
		filter,
		retryKey,
	);

	const applyFilter = () => {
		const next = keyword.trim();
		setFilter(next);
		if (next) setSearchParams({ task_id: next }, { replace: true });
		else setSearchParams({}, { replace: true });
	};

	const columns: TableProps<HistoryEvent>["columns"] = [
		{
			title: "时间",
			key: "timestamp",
			render: (_, event) => formatTimestamp(event.timestamp),
		},
		{ title: "任务", dataIndex: "task_id", key: "task_id" },
		{
			title: "事件",
			key: "type",
			render: (_, event) => (
				<Tag
					color={
						event.type === "failed"
							? "red"
							: event.type === "completed"
								? "green"
								: undefined
					}
				>
					{EVENT_LABELS[event.type] ?? event.type}
				</Tag>
			),
		},
		{ title: "路径", key: "path", render: (_, event) => event.path || "" },
		{
			title: "结果·错误",
			key: "result",
			render: (_, event) => resultText(event),
		},
	];

	return (
		<section className="management-page">
			<header className="management-heading">
				<div>
					<h2>运行日志</h2>
					<Typography.Text type="secondary">
						实时查看任务执行事件与处理结果
					</Typography.Text>
				</div>
				<span className={`connection-state ${connected ? "is-connected" : ""}`}>
					{loading
						? "正在加载运行日志…"
						: error
							? "运行日志连接失败"
							: connected
								? "实时连接已建立"
								: "实时连接重试中…"}
				</span>
			</header>
			{error ? (
				<Alert
					type="error"
					showIcon
					message="运行日志加载失败"
					description={error}
					action={
						<Button onClick={() => setRetryKey((value) => value + 1)}>
							重新加载
						</Button>
					}
				/>
			) : null}
			<Space className="management-toolbar" wrap>
				<Input
					placeholder="任务 ID（留空显示全部）"
					value={keyword}
					onChange={(e) => setKeyword(e.target.value)}
					onPressEnter={applyFilter}
					style={{ width: 240 }}
					allowClear
				/>
				<Button type="primary" onClick={applyFilter}>
					筛选
				</Button>
			</Space>
			<Table<HistoryEvent>
				rowKey="id"
				pagination={false}
				columns={columns}
				dataSource={events}
				locale={{
					emptyText: error
						? "日志暂时无法读取，请重新加载"
						: filter
							? "该任务暂无运行日志"
							: "暂无运行日志",
				}}
				scroll={{ x: 760 }}
			/>
		</section>
	);
}
