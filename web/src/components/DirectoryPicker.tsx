import { useCallback, useEffect, useRef, useState } from "react";
import {
	App,
	Breadcrumb,
	Button,
	Empty,
	Input,
	List,
	Modal,
	Space,
	Spin,
	Typography,
} from "antd";
import {
	FolderAddOutlined,
	FolderOpenOutlined,
	FolderOutlined,
} from "@ant-design/icons";
import { getJSON, postJSON } from "../api/client";
import type { LocalDirectoryListing, StorageEntry } from "../api/types";

export type DirectoryPickerMode = "storage" | "local";

interface DirectoryPickerProps {
	open: boolean;
	mode: DirectoryPickerMode;
	title: string;
	/** Required for mode="storage"; identifies which storage to browse. */
	storageID?: string;
	/** Directory the picker opens at. Falls back to the mode's root. */
	value: string;
	onCancel: () => void;
	onSelect: (path: string) => void;
}

interface Level {
	path: string;
	parent: string;
	directories: { name: string; path: string }[];
}

function joinStoragePath(parent: string, name: string): string {
	return parent === "/" ? `/${name}` : `${parent}/${name}`;
}

function storageParent(path: string): string {
	const parts = path.replace(/\/+$/, "").split("/");
	parts.pop();
	return parts.join("/") || "/";
}

function crumbsOf(path: string): { label: string; path: string }[] {
	const segments = path.split("/").filter(Boolean);
	const crumbs = [{ label: "根目录", path: "/" }];
	let current = "";
	for (const segment of segments) {
		current += `/${segment}`;
		crumbs.push({ label: segment, path: current });
	}
	return crumbs;
}

export default function DirectoryPicker({
	open,
	mode,
	title,
	storageID,
	value,
	onCancel,
	onSelect,
}: DirectoryPickerProps) {
	const { message } = App.useApp();
	const [level, setLevel] = useState<Level | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState("");
	const [creating, setCreating] = useState(false);
	const [newName, setNewName] = useState("");
	const browseRequest = useRef(0);

	const browse = useCallback(
		async (path: string) => {
			const request = ++browseRequest.current;
			setLoading(true);
			setError("");
			try {
				if (mode === "local") {
					const listing = await getJSON<LocalDirectoryListing>(
						`/api/local-dirs?path=${encodeURIComponent(path)}`,
					);
					if (request !== browseRequest.current) return;
					setLevel({
						path: listing.path,
						parent: listing.parent,
						directories: listing.directories ?? [],
					});
				} else {
					const entries = await getJSON<StorageEntry[]>(
						`/api/storages/${encodeURIComponent(storageID ?? "")}/entries?path=${encodeURIComponent(path)}`,
					);
					if (request !== browseRequest.current) return;
					setLevel({
						path,
						parent: path === "/" ? "" : storageParent(path),
						directories: (entries ?? [])
							.filter((entry) => entry.is_dir)
							.map((entry) => ({
								name: entry.name,
								path: entry.path || joinStoragePath(path, entry.name),
							})),
					});
				}
			} catch (browseError) {
				if (request !== browseRequest.current) return;
				setError(
					browseError instanceof Error ? browseError.message : "目录读取失败",
				);
			} finally {
				if (request === browseRequest.current) setLoading(false);
			}
		},
		[mode, storageID],
	);

	useEffect(() => {
		if (!open) {
			browseRequest.current += 1;
			setLevel(null);
			setLoading(false);
			return;
		}
		setLevel(null);
		setCreating(false);
		setNewName("");
		void browse(value?.trim() || "/");
		return () => {
			browseRequest.current += 1;
		};
	}, [browse, open, value]);

	const createDirectory = async () => {
		const name = newName.trim();
		if (!name || !level) return;
		try {
			if (mode === "local") {
				await postJSON("/api/local-dirs", { path: level.path, name });
			} else {
				await postJSON(
					`/api/storages/${encodeURIComponent(storageID ?? "")}/mkdir`,
					{
						path: joinStoragePath(level.path, name),
					},
				);
			}
			void message.success("目录已创建");
			setCreating(false);
			setNewName("");
			await browse(level.path);
		} catch (createError) {
			void message.error(
				createError instanceof Error ? createError.message : "创建目录失败",
			);
		}
	};

	return (
		<Modal
			title={title}
			open={open}
			width={620}
			okText="选择当前目录"
			cancelText="取消"
			okButtonProps={{ disabled: !level }}
			onOk={() => level && onSelect(level.path)}
			onCancel={onCancel}
			destroyOnHidden
		>
			<Space direction="vertical" size="middle" style={{ width: "100%" }}>
				<Space wrap>
					<Button
						disabled={!level?.parent || loading}
						onClick={() => level?.parent && void browse(level.parent)}
					>
						上一级
					</Button>
					<Button
						icon={<FolderAddOutlined />}
						disabled={!level || loading}
						onClick={() => setCreating(true)}
					>
						新建目录
					</Button>
				</Space>

				{mode === "storage" && level ? (
					<Breadcrumb
						items={crumbsOf(level.path).map((crumb) => ({
							title: (
								<Typography.Link onClick={() => void browse(crumb.path)}>
									{crumb.label}
								</Typography.Link>
							),
						}))}
					/>
				) : null}

				<Input
					readOnly
					value={level?.path ?? ""}
					aria-label="当前目录"
					prefix={<FolderOpenOutlined />}
				/>

				{creating ? (
					<Space.Compact style={{ width: "100%" }}>
						<Input
							autoFocus
							aria-label="新目录名称"
							placeholder="新目录名称"
							value={newName}
							onChange={(event) => setNewName(event.target.value)}
							onPressEnter={() => void createDirectory()}
						/>
						<Button
							type="primary"
							aria-label="创建"
							onClick={() => void createDirectory()}
						>
							创建
						</Button>
						<Button
							aria-label="取消新建"
							onClick={() => {
								setCreating(false);
								setNewName("");
							}}
						>
							取消
						</Button>
					</Space.Compact>
				) : null}

				{error ? (
					<Typography.Text type="danger">{error}</Typography.Text>
				) : null}

				<Spin spinning={loading}>
					<div className="directory-picker-list">
						<List
							size="small"
							bordered
							dataSource={level?.directories ?? []}
							locale={{
								emptyText: (
									<Empty description="这里没有子目录，可直接选择当前目录" />
								),
							}}
							renderItem={(item) => (
								<List.Item>
									<Typography.Link onClick={() => void browse(item.path)}>
										<Space size={6}>
											<FolderOutlined />
											{item.name}
										</Space>
									</Typography.Link>
								</List.Item>
							)}
						/>
					</div>
				</Spin>
			</Space>
		</Modal>
	);
}
