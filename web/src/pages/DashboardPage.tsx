import { useCallback, useState } from "react";
import { NavLink } from "react-router";
import {
	Alert,
	App,
	Button,
	Card,
	Empty,
	Space,
	Spin,
	Statistic,
	Table,
	Tag,
	Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
	ClockCircleOutlined,
	CloudServerOutlined,
	ExclamationCircleOutlined,
	PlayCircleOutlined,
	UnorderedListOutlined,
} from "@ant-design/icons";
import type { ReactNode } from "react";
import { getOverview } from "../api/bootstrap";
import type { HistoryEvent, Overview } from "../api/types";
import { formatTimestamp } from "../format";
import { usePolling } from "../hooks/usePolling";
import "./DashboardPage.css";

function StatCard({
	label,
	value,
	icon,
	tone,
}: {
	label: string;
	value: number;
	icon: ReactNode;
	tone?: "success" | "danger";
}) {
	return (
		<Card className={`dashboard-stat${tone ? ` is-${tone}` : ""}`}>
			<span className="dashboard-stat-label">
				<span className="dashboard-stat-icon" aria-hidden="true">
					{icon}
				</span>
				{label}
			</span>
			<Statistic value={value} />
		</Card>
	);
}

const eventLabel = (type: string) =>
	({
		queued: "已排队",
		started: "已开始",
		completed: "已完成",
		failed: "失败",
		stopped: "已停止",
	})[type] ?? type;

const historyColumns: ColumnsType<HistoryEvent> = [
	{
		title: "时间",
		dataIndex: "timestamp",
		width: 196,
		className: "dashboard-time-cell",
		render: (value) => formatTimestamp(value),
	},
	{ title: "任务", dataIndex: "task_id", width: 140 },
	{
		title: "事件",
		dataIndex: "type",
		width: 110,
		render: (type: string) => (
			<Tag
				color={
					type === "failed" ? "red" : type === "completed" ? "green" : undefined
				}
			>
				{eventLabel(type)}
			</Tag>
		),
	},
	{
		title: "结果",
		render: (_, event) =>
			event.error ||
			[
				`新增 ${event.created ?? 0}`,
				`复制 ${event.copied ?? 0}`,
				`删除 ${event.removed ?? 0}`,
				`跳过 ${event.skipped ?? 0}`,
				`失败 ${event.failed ?? 0}`,
			].join(" · "),
	},
];

export default function DashboardPage() {
	const { message } = App.useApp();
	const [overview, setOverview] = useState<Overview | null>(null);
	const [error, setError] = useState("");

	const refresh = useCallback(async () => {
		try {
			setOverview(await getOverview());
			setError("");
		} catch (loadError) {
			const text =
				loadError instanceof Error ? loadError.message : "仪表盘加载失败";
			setError(text);
			void message.error(text);
		}
	}, [message]);

	usePolling(refresh, 5000);

	if (!overview && !error)
		return (
			<Spin tip="加载仪表盘">
				<div style={{ minHeight: 160 }} />
			</Spin>
		);

	return (
		<Space
			className="dashboard-page"
			direction="vertical"
			size="large"
			style={{ width: "100%" }}
		>
			<div className="dashboard-heading">
				<div>
					<Typography.Title level={2}>仪表盘</Typography.Title>
					<Typography.Text type="secondary">
						任务、存储与最近运行概览
					</Typography.Text>
				</div>
				<Space wrap>
					<Button type="primary">
						<NavLink to="/tasks">运行任务</NavLink>
					</Button>
					<Button>
						<NavLink to="/storage">浏览存储</NavLink>
					</Button>
					<Button>
						<NavLink to="/config">修改配置</NavLink>
					</Button>
				</Space>
			</div>
			{error ? <Alert type="error" showIcon message={error} /> : null}
			{overview ? (
				<>
					{overview.storages === 0 || overview.tasks.total === 0 ? (
						<Alert
							type="info"
							showIcon
							message="欢迎使用 SmartStrm"
							description={
								overview.storages === 0
									? "第一步是添加媒体存储，让 SmartStrm 知道从哪里读取文件。连接存储不会移动或删除原文件。"
									: "存储已经准备好。下一步创建 STRM 任务，选择要扫描的目录和输出位置。"
							}
							action={
								<Button type="primary">
									<NavLink
										to={
											overview.storages === 0
												? "/config?section=storages"
												: "/config?section=tasks&action=add-task"
										}
									>
										{overview.storages === 0
											? "开始添加存储"
											: "创建第一个任务"}
									</NavLink>
								</Button>
							}
						/>
					) : null}
					<div className="dashboard-stats">
						<StatCard
							label="任务总数"
							value={overview.tasks.total}
							icon={<UnorderedListOutlined />}
						/>
						<StatCard
							label="运行中"
							value={overview.tasks.running}
							icon={<PlayCircleOutlined />}
							tone={overview.tasks.running > 0 ? "success" : undefined}
						/>
						<StatCard
							label="失败"
							value={overview.tasks.failed}
							icon={<ExclamationCircleOutlined />}
							tone={overview.tasks.failed > 0 ? "danger" : undefined}
						/>
						<StatCard
							label="队列"
							value={overview.tasks.queued}
							icon={<ClockCircleOutlined />}
						/>
						<StatCard
							label="存储总数"
							value={overview.storages}
							icon={<CloudServerOutlined />}
						/>
					</div>
					<Card className="dashboard-history" title="最近运行">
						<Table
							rowKey="id"
							columns={historyColumns}
							dataSource={(overview.history ?? []).slice(0, 10)}
							pagination={false}
							locale={{ emptyText: <Empty description="暂无运行记录" /> }}
							scroll={(overview.history?.length ?? 0) > 0 ? { x: 700 } : undefined}
						/>
					</Card>
				</>
			) : null}
		</Space>
	);
}
