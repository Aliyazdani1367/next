import {
	Alert,
	AlertIcon,
	Badge,
	Button,
	FormControl,
	FormLabel,
	HStack,
	Progress,
	Stack,
	Text,
	useToast,
} from "@chakra-ui/react";
import { ArrowUpTrayIcon } from "@heroicons/react/24/outline";
import { PanelSelect as Select } from "components/common/PanelSelect";
import { useServicesStore } from "contexts/ServicesContext";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation } from "react-query";
import {
	importUserMigration,
	inspectUserMigrationBackup,
	type UserMigrationInspectResponse,
} from "service/settings";
import {
	generateErrorMessage,
	generateSuccessMessage,
} from "utils/toastHandler";
import { FileDropzone } from "./common/FileDropzone";

export const UserMigrationPanel = () => {
	const { t } = useTranslation();
	const toast = useToast();
	const [selectedFile, setSelectedFile] = useState<File | null>(null);
	const [uploadProgress, setUploadProgress] = useState<number | null>(null);
	const [inspected, setInspected] =
		useState<UserMigrationInspectResponse | null>(null);
	const [sourceAdmin, setSourceAdmin] = useState<string>("");
	const [serviceId, setServiceId] = useState<number | null>(null);

	const services = useServicesStore((state) => state.serviceOptions);
	const servicesLoading = useServicesStore((state) => state.isOptionsLoading);
	const fetchServiceOptions = useServicesStore(
		(state) => state.fetchServiceOptions,
	);

	useEffect(() => {
		fetchServiceOptions({ limit: 1000 });
	}, [fetchServiceOptions]);

	useEffect(() => {
		if (serviceId === null && services.length) {
			setServiceId(services[0].id);
		}
	}, [services, serviceId]);

	const inspectMutation = useMutation(
		(file: File) => inspectUserMigrationBackup(file, setUploadProgress),
		{
			onMutate: () => setUploadProgress(0),
			onSuccess: (result) => {
				setInspected(result);
				setSourceAdmin(result.admins[0]?.username ?? "");
			},
			onError: (error) => {
				generateErrorMessage(error, toast);
			},
			onSettled: () => setUploadProgress(null),
		},
	);

	const importMutation = useMutation(importUserMigration, {
		onSuccess: (result) => {
			generateSuccessMessage(
				t("myaccount.importUsers.resultSummary", {
					imported: result.imported,
					total: result.total,
				}),
				toast,
			);
		},
		onError: (error) => {
			generateErrorMessage(error, toast);
		},
	});

	const reset = () => {
		setSelectedFile(null);
		setInspected(null);
		setSourceAdmin("");
		importMutation.reset();
	};

	const handleInspect = () => {
		if (!selectedFile) {
			toast({
				status: "warning",
				title: t("myaccount.importUsers.fileRequired"),
			});
			return;
		}
		inspectMutation.mutate(selectedFile);
	};

	const handleImport = () => {
		if (!inspected || !sourceAdmin || !serviceId) return;
		importMutation.mutate({
			token: inspected.token,
			source_admin_username: sourceAdmin,
			service_id: serviceId,
		});
	};

	const result = importMutation.data;

	return (
		<Stack
			spacing={4}
			w="full"
			maxW="720px"
			mx="auto"
			bg="panel.surface"
			borderWidth="1px"
			borderColor="panel.border"
			borderRadius="2xl"
			p={{ base: 4, md: 6 }}
		>
			<Stack spacing={1}>
				<Text fontWeight="700" fontSize="15px">
					{t("myaccount.importUsers.title")}
				</Text>
				<Text fontSize="13px" color="panel.textMuted">
					{t("myaccount.importUsers.description")}
				</Text>
			</Stack>

			{!inspected && !result && (
				<Stack spacing={4}>
					<FormControl>
						<FormLabel fontSize="13px" fontWeight="600" color="panel.textSecondary">
							{t("myaccount.importUsers.file")}
						</FormLabel>
						<FileDropzone
							accept=".rbbackup,.sqlite3,.sqlite,.db,.sql,application/vnd.next.backup,application/gzip"
							isDisabled={inspectMutation.isLoading}
							selectedFile={selectedFile}
							title={t("myaccount.importUsers.dropTitle")}
							description={t("myaccount.importUsers.dropHint")}
							emptyText={t("myaccount.importUsers.selectFile")}
							onFileSelect={setSelectedFile}
						/>
					</FormControl>
					{inspectMutation.isLoading && uploadProgress !== null && (
						<Stack spacing={2} aria-live="polite">
							<Text fontSize="12px" fontWeight="600">
								{uploadProgress < 100
									? t("myaccount.importUsers.uploadProgress", {
											percent: uploadProgress,
										})
									: t("myaccount.importUsers.inspecting")}
							</Text>
							<Progress
								value={uploadProgress}
								isIndeterminate={uploadProgress >= 100}
								colorScheme="primary"
								borderRadius="full"
								size="xs"
								h="4px"
							/>
						</Stack>
					)}
					<HStack justify="flex-end">
						<Button
							colorScheme="primary"
							size="sm"
							borderRadius="full"
							px={5}
							leftIcon={<ArrowUpTrayIcon width={15} height={15} />}
							onClick={handleInspect}
							isLoading={inspectMutation.isLoading}
						>
							{t("myaccount.importUsers.uploadButton")}
						</Button>
					</HStack>
				</Stack>
			)}

			{inspected && !result && (
				<Stack spacing={4}>
					{inspected.admins.length === 0 ? (
						<Alert status="warning" borderRadius="xl" fontSize="13px">
							<AlertIcon />
							<Text fontSize="13px">
								{t("myaccount.importUsers.noAdminsFound")}
							</Text>
						</Alert>
					) : (
						<>
							<FormControl>
								<FormLabel fontSize="13px" fontWeight="600" color="panel.textSecondary">
									{t("myaccount.importUsers.chooseAdmin")}
								</FormLabel>
								<Select
									value={sourceAdmin}
									showSearch={false}
									onChange={(event) => setSourceAdmin(event.target.value)}
								>
									{inspected.admins.map((admin) => (
										<option key={admin.username} value={admin.username}>
											{admin.username} ({admin.user_count})
										</option>
									))}
								</Select>
								<Text fontSize="12px" color="panel.textMuted" mt={1}>
									{t("myaccount.importUsers.chooseAdminHint")}
								</Text>
							</FormControl>
							<FormControl>
								<FormLabel fontSize="13px" fontWeight="600" color="panel.textSecondary">
									{t("myaccount.importUsers.destinationService")}
								</FormLabel>
								<Select
									value={serviceId ?? ""}
									showSearch={false}
									isDisabled={servicesLoading || services.length === 0}
									onChange={(event) =>
										setServiceId(Number(event.target.value) || null)
									}
								>
									{services.map((service) => (
										<option key={service.id} value={service.id}>
											{service.name}
										</option>
									))}
								</Select>
								<Text fontSize="12px" color="panel.textMuted" mt={1}>
									{t("myaccount.importUsers.destinationServiceHint")}
								</Text>
							</FormControl>
						</>
					)}
					<HStack justify="flex-end" spacing={2}>
						<Button
							variant="ghost"
							size="sm"
							borderRadius="full"
							onClick={reset}
							isDisabled={importMutation.isLoading}
						>
							{t("cancel")}
						</Button>
						{inspected.admins.length > 0 && (
							<Button
								colorScheme="primary"
								size="sm"
								borderRadius="full"
								px={5}
								onClick={handleImport}
								isLoading={importMutation.isLoading}
								isDisabled={!sourceAdmin || !serviceId}
							>
								{t("myaccount.importUsers.importButton")}
							</Button>
						)}
					</HStack>
				</Stack>
			)}

			{result && (
				<Stack spacing={3}>
					<Alert
						status={result.skipped.length ? "warning" : "success"}
						borderRadius="xl"
						fontSize="13px"
					>
						<AlertIcon />
						<Text fontSize="13px">
							{t("myaccount.importUsers.resultSummary", {
								imported: result.imported,
								total: result.total,
							})}
						</Text>
					</Alert>
					{result.skipped.length > 0 && (
						<Stack
							spacing={2}
							borderWidth="1px"
							borderColor="panel.border"
							borderRadius="xl"
							p={3}
						>
							<Text fontSize="13px" fontWeight="600">
								{t("myaccount.importUsers.skippedTitle", {
									count: result.skipped.length,
								})}
							</Text>
							{result.skipped.map((skip) => (
								<HStack key={skip.username} spacing={2} align="flex-start">
									<Badge colorScheme="orange" flexShrink={0}>
										{skip.username}
									</Badge>
									<Text fontSize="12px" color="panel.textMuted">
										{skip.reason}
									</Text>
								</HStack>
							))}
						</Stack>
					)}
					<HStack justify="flex-end">
						<Button
							variant="outline"
							size="sm"
							borderRadius="full"
							onClick={reset}
						>
							{t("myaccount.importUsers.importAnother")}
						</Button>
					</HStack>
				</Stack>
			)}
		</Stack>
	);
};
