import { Button, Empty, Input, Space, Table, Typography } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";

export interface MappingRow {
	from: string;
	to: string;
}

interface PathMappingFieldProps {
	id?: string;
	rows: MappingRow[];
	onChange: (rows: MappingRow[]) => void;
	fromLabel: string;
	toLabel: string;
	fromPlaceholder: string;
	toPlaceholder: string;
	addLabel: string;
}

export function rowsFromRecord(
	record: Record<string, string> | null | undefined,
): MappingRow[] {
	return Object.entries(record ?? {}).map(([from, to]) => ({ from, to }));
}

/** Later duplicates win, matching plain object literal semantics. */
export function recordFromRows(rows: MappingRow[]): Record<string, string> {
	const record: Record<string, string> = {};
	for (const row of rows) {
		const from = row.from.trim();
		if (from) record[from] = row.to.trim();
	}
	return record;
}

export default function PathMappingField({
	id,
	rows,
	onChange,
	fromLabel,
	toLabel,
	fromPlaceholder,
	toPlaceholder,
	addLabel,
}: PathMappingFieldProps) {
	const update = (index: number, patch: Partial<MappingRow>) => {
		onChange(
			rows.map((row, rowIndex) =>
				rowIndex === index ? { ...row, ...patch } : row,
			),
		);
	};

	return (
		<div id={id}>
			<Table<MappingRow>
				size="small"
				pagination={false}
				rowKey={(_, index) => String(index)}
				dataSource={rows}
				locale={{
					emptyText: (
						<Empty
							image={Empty.PRESENTED_IMAGE_SIMPLE}
							description="尚未配置映射"
						/>
					),
				}}
				columns={[
					{
						title: fromLabel,
						render: (_, row, index) => (
							<Input
								value={row.from}
								placeholder={fromPlaceholder}
								aria-label={`${fromLabel} ${index + 1}`}
								onChange={(event) =>
									update(index, { from: event.target.value })
								}
							/>
						),
					},
					{
						title: toLabel,
						render: (_, row, index) => (
							<Input
								value={row.to}
								placeholder={toPlaceholder}
								aria-label={`${toLabel} ${index + 1}`}
								onChange={(event) => update(index, { to: event.target.value })}
							/>
						),
					},
					{
						title: "",
						width: 56,
						render: (_, _row, index) => (
							<Button
								type="text"
								danger
								icon={<DeleteOutlined />}
								aria-label={`删除${fromLabel} ${index + 1}`}
								onClick={() =>
									onChange(rows.filter((__, rowIndex) => rowIndex !== index))
								}
							/>
						),
					},
				]}
			/>
			<Space style={{ marginTop: 8 }}>
				<Button
					icon={<PlusOutlined />}
					onClick={() => onChange([...rows, { from: "", to: "" }])}
				>
					{addLabel}
				</Button>
				<Typography.Text type="secondary">
					留空的行会在保存时忽略
				</Typography.Text>
			</Space>
		</div>
	);
}
