import {
	Alert,
	AlertDescription,
	AlertIcon,
	Box,
	Button,
	chakra,
	FormControl,
	FormErrorMessage,
	FormLabel,
	HStack,
	IconButton,
	Input as CInput,
	InputGroup,
	InputLeftElement,
	InputRightElement,
	Menu,
	MenuButton,
	MenuItem,
	MenuList,
	Portal,
	Text,
	useColorMode,
	useColorModeValue,
	VStack,
} from "@chakra-ui/react";
import {
	ArrowRightOnRectangleIcon,
	CheckIcon,
	EyeIcon,
	EyeSlashIcon,
	LockClosedIcon,
	MoonIcon,
	SunIcon,
	UserIcon,
} from "@heroicons/react/24/outline";
import { zodResolver } from "@hookform/resolvers/zod";
import { keyframes } from "@emotion/react";
import logoUrl from "assets/logo.svg";
import { Language } from "components/Language";
import { motion } from "framer-motion";
import {
	type FC,
	type MouseEvent as ReactMouseEvent,
	type ReactElement,
	type ReactNode,
	useEffect,
	useState,
} from "react";
import {
	type FieldErrors,
	useForm,
	type UseFormRegisterReturn,
} from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { QRCodeCanvas } from "qrcode.react";
import {
	confirm2FASetup,
	getSession,
	login as createSession,
	logout,
	start2FASetup,
	type TOTPSetup,
	verify2FA,
} from "service/auth";
import { clearClientSession } from "utils/session";
import { updateThemeColor } from "utils/themeColor";
import { z } from "zod";

const schema = z.object({
	username: z.string().min(1, "login.fieldRequired"),
	password: z.string().min(1, "login.fieldRequired"),
});

export const LogoIcon = chakra("img", {
	baseStyle: {
		h: 8,
		w: 8,
	},
});

const LoginIcon = chakra(ArrowRightOnRectangleIcon, {
	baseStyle: {
		h: 5,
		strokeWidth: "2px",
		w: 5,
	},
});

const Eye = chakra(EyeIcon, { baseStyle: { h: 4, w: 4 } });
const EyeSlash = chakra(EyeSlashIcon, { baseStyle: { h: 4, w: 4 } });
const User = chakra(UserIcon, {
	baseStyle: { h: 5, strokeWidth: "1.8px", w: 5 },
});
const Lock = chakra(LockClosedIcon, {
	baseStyle: { h: 5, strokeWidth: "1.8px", w: 5 },
});
const Moon = chakra(MoonIcon, { baseStyle: { h: 4, w: 4 } });
const Sun = chakra(SunIcon, { baseStyle: { h: 4, w: 4 } });
const Check = chakra(CheckIcon, { baseStyle: { h: 4, w: 4 } });

const MotionBox = motion(Box);

const floatBlobA = keyframes`
	0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
	50% { transform: translate3d(4%, -6%, 0) scale(1.12); }
`;
const floatBlobB = keyframes`
	0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
	50% { transform: translate3d(-5%, 5%, 0) scale(1.08); }
`;
const floatBlobC = keyframes`
	0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
	50% { transform: translate3d(3%, 4%, 0) scale(0.94); }
`;
const glowPulse = keyframes`
	0%, 100% { opacity: 0.5; transform: translate(-50%, -50%) scale(1); }
	50% { opacity: 0.8; transform: translate(-50%, -50%) scale(1.08); }
`;
const logoPulse = keyframes`
	0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--rb-panel-accent) 45%, transparent); }
	50% { box-shadow: 0 0 0 10px color-mix(in srgb, var(--rb-panel-accent) 0%, transparent); }
`;

const THEME_KEY = "rb-theme";
const CHAKRA_THEME_KEY = "chakra-ui-color-mode";
const CUSTOM_THEMES_KEY = "rb-custom-themes";

type LoginThemeMode = "dark" | "light";

type LoginFormValues = {
	username: string;
	password: string;
};

type LoginFieldProps = {
	autoComplete: string;
	dir: "ltr" | "rtl";
	endElement?: ReactNode;
	errorMessage?: string;
	icon: ReactNode;
	label: string;
	placeholder: string;
	registration: UseFormRegisterReturn;
	type?: string;
};

const LoginField: FC<LoginFieldProps> = ({
	autoComplete,
	dir,
	endElement,
	errorMessage,
	icon,
	label,
	placeholder,
	registration,
	type = "text",
}) => {
	const isInvalid = Boolean(errorMessage);
	const fieldBg = useColorModeValue("white", "var(--rb-panel-main)");
	const borderColor = useColorModeValue(
		"var(--rb-panel-border)",
		"var(--rb-panel-border)",
	);
	const textColor = useColorModeValue(
		"var(--rb-panel-text)",
		"var(--rb-panel-text)",
	);
	const mutedColor = useColorModeValue(
		"var(--rb-panel-text-muted)",
		"var(--rb-panel-text-muted)",
	);

	return (
		<FormControl isInvalid={isInvalid}>
			<FormLabel
				color={textColor}
				fontSize="sm"
				fontWeight="700"
				letterSpacing="0"
				mb={2}
			>
				{label}
			</FormLabel>
			<InputGroup dir={dir}>
				<InputLeftElement color={isInvalid ? "red.400" : mutedColor} h="44px">
					{icon}
				</InputLeftElement>
				<CInput
					{...registration}
					autoComplete={autoComplete}
					bg={fieldBg}
					borderColor={isInvalid ? "red.400" : borderColor}
					borderRadius="8px"
					color={textColor}
					fontSize="sm"
					h="44px"
					pe={endElement ? "3rem" : 4}
					placeholder={placeholder}
					ps="3rem"
					type={type}
					_placeholder={{ color: mutedColor }}
					_hover={{
						borderColor: isInvalid
							? "red.400"
							: "var(--rb-panel-border-strong)",
					}}
					_focusVisible={{
						borderColor: isInvalid ? "red.400" : "var(--rb-panel-accent)",
						boxShadow: isInvalid
							? "0 0 0 1px rgba(248, 113, 113, 0.6)"
							: "0 0 0 1px var(--rb-panel-accent)",
					}}
				/>
				{endElement && (
					<InputRightElement color={mutedColor} h="44px">
						{endElement}
					</InputRightElement>
				)}
			</InputGroup>
			<FormErrorMessage fontSize="xs">{errorMessage}</FormErrorMessage>
		</FormControl>
	);
};

const applyLoginThemeMode = (theme: LoginThemeMode) => {
	try {
		localStorage.setItem(THEME_KEY, theme);
		localStorage.setItem(CHAKRA_THEME_KEY, theme);
		localStorage.removeItem(CUSTOM_THEMES_KEY);
	} catch {}

	const targets = [document.documentElement, document.body].filter(
		Boolean,
	) as HTMLElement[];
	targets.forEach((target) => {
		target.classList.remove(
			"rb-theme-light",
			"rb-theme-dark",
			"chakra-ui-light",
			"chakra-ui-dark",
		);
		target.classList.add(`rb-theme-${theme}`, `chakra-ui-${theme}`);
		target.dataset.theme = theme;
		target.style.colorScheme = theme;
	});
	updateThemeColor(theme);
};

const LoginThemeMenu: FC = () => {
	const { t } = useTranslation();
	const { colorMode, setColorMode } = useColorMode();
	const activeTheme = colorMode === "light" ? "light" : "dark";
	const menuBg = useColorModeValue("panel.surface", "panel.surface");
	const menuBorder = useColorModeValue("panel.border", "panel.border");
	const hoverBg = useColorModeValue("panel.elevated", "panel.elevated");
	const textColor = useColorModeValue("panel.text", "panel.text");

	const selectTheme = (theme: LoginThemeMode) => {
		applyLoginThemeMode(theme);
		setColorMode(theme);
	};

	const options: Array<{
		key: LoginThemeMode;
		label: string;
		icon: ReactElement;
	}> = [
		{ key: "dark", label: t("theme.dark"), icon: <Moon /> },
		{ key: "light", label: t("theme.light"), icon: <Sun /> },
	];

	return (
		<Menu placement="bottom-end" strategy="fixed" autoSelect={false}>
			<MenuButton
				as={IconButton}
				aria-label={t("header.theme")}
				icon={activeTheme === "dark" ? <Moon /> : <Sun />}
				size="sm"
				variant="ghost"
			/>
			<Portal>
				<MenuList
					bg={menuBg}
					borderColor={menuBorder}
					color={textColor}
					minW="150px"
					p={1}
				>
					{options.map((option) => (
						<MenuItem
							key={option.key}
							icon={option.icon}
							onClick={() => selectTheme(option.key)}
							_hover={{ bg: hoverBg }}
						>
							<HStack justify="space-between" w="full">
								<Text>{option.label}</Text>
								{activeTheme === option.key ? <Check /> : null}
							</HStack>
						</MenuItem>
					))}
				</MenuList>
			</Portal>
		</Menu>
	);
};

export const Login: FC = () => {
	const [error, setError] = useState("");
	const [showPassword, setShowPassword] = useState(false);
	const [step, setStep] = useState<"credentials" | "otp" | "setup">(
		"credentials",
	);
	const [otp, setOTP] = useState("");
	const [setup, setSetup] = useState<TOTPSetup | null>(null);
	const [challengeLoading, setChallengeLoading] = useState(false);
	const navigate = useNavigate();
	const { t, i18n } = useTranslation();
	const dir = i18n.language === "fa" ? "rtl" : "ltr";
	const pageBg = useColorModeValue(
		"var(--rb-panel-main)",
		"var(--rb-panel-main)",
	);
	const surfaceBg = useColorModeValue(
		"var(--rb-panel-surface)",
		"var(--rb-panel-surface)",
	);
	const elevatedBg = useColorModeValue(
		"var(--rb-panel-elevated)",
		"var(--rb-panel-elevated)",
	);
	const borderColor = useColorModeValue(
		"var(--rb-panel-border)",
		"var(--rb-panel-border)",
	);
	const textColor = useColorModeValue(
		"var(--rb-panel-text)",
		"var(--rb-panel-text)",
	);
	const mutedColor = useColorModeValue(
		"var(--rb-panel-text-muted)",
		"var(--rb-panel-text-muted)",
	);
	const accentColor = "var(--rb-panel-accent)";

	const {
		register,
		formState: { errors, isSubmitting },
		handleSubmit,
		watch,
	} = useForm<LoginFormValues>({
		resolver: zodResolver(schema),
		defaultValues: {
			password: "",
			username: "",
		},
	});

	const usernameValue = watch("username") || "";
	const passwordValue = watch("password") || "";
	const canSubmit =
		Boolean(usernameValue.trim().length) &&
		Boolean(passwordValue.trim().length) &&
		!isSubmitting;

	useEffect(() => {
		void usernameValue;
		void passwordValue;
		setError((current) => (current ? "" : current));
	}, [usernameValue, passwordValue]);

	useEffect(() => {
		getSession()
			.then(async (session) => {
				if (session.state === "active") {
					navigate("/");
				} else if (session.state === "disabled") {
					navigate("/users");
				} else if (session.state === "pending_2fa") {
					setStep("otp");
				} else {
					setSetup(await start2FASetup());
					setStep("setup");
				}
			})
			.catch(() => undefined);
	}, [navigate]);

	const completeLogin = () => {
		clearClientSession();
		navigate("/");
	};

	const openRequiredSetup = async () => {
		setSetup(await start2FASetup());
		setStep("setup");
	};

	const login = async (values: LoginFormValues) => {
		setError("");
		try {
			const session = await createSession(values.username, values.password);
			if (session.state === "pending_2fa") {
				setStep("otp");
				setOTP("");
			} else if (session.state === "setup_required") {
				await openRequiredSetup();
			} else if (session.state === "disabled") {
				clearClientSession();
				navigate("/users");
			} else {
				completeLogin();
			}
		} catch (err: any) {
			setError(err.response?._data?.detail || "Login failed");
		}
	};

	const submitOTP = async () => {
		setError("");
		setChallengeLoading(true);
		try {
			await verify2FA(otp);
			completeLogin();
		} catch (err: any) {
			setError(err.response?._data?.detail || "Invalid authentication code");
		} finally {
			setChallengeLoading(false);
		}
	};

	const confirmSetup = async () => {
		setError("");
		setChallengeLoading(true);
		try {
			await confirm2FASetup(otp);
			completeLogin();
		} catch (err: any) {
			setError(err.response?._data?.detail || "Invalid authentication code");
		} finally {
			setChallengeLoading(false);
		}
	};

	const cancelChallenge = async () => {
		try {
			await logout();
		} finally {
			setStep("credentials");
			setOTP("");
			setSetup(null);
			setError("");
		}
	};

	const handleInvalid = async (_errors: FieldErrors<LoginFormValues>) => {
		setError("");
	};

	const [tilt, setTilt] = useState({ rx: 0, ry: 0 });
	const handleCardMouseMove = (event: ReactMouseEvent<HTMLDivElement>) => {
		const rect = event.currentTarget.getBoundingClientRect();
		const px = (event.clientX - rect.left) / rect.width;
		const py = (event.clientY - rect.top) / rect.height;
		setTilt({ rx: (0.5 - py) * 8, ry: (px - 0.5) * 8 });
	};
	const handleCardMouseLeave = () => setTilt({ rx: 0, ry: 0 });
	const blobOpacity = useColorModeValue(0.35, 0.55);

	const passwordToggle = (
		<IconButton
			aria-label={
				showPassword ? t("admins.hidePassword") : t("admins.showPassword")
			}
			color={mutedColor}
			icon={showPassword ? <EyeSlash /> : <Eye />}
			onClick={() => setShowPassword((visible) => !visible)}
			onMouseDown={(event) => event.preventDefault()}
			size="sm"
			variant="ghost"
			_hover={{ bg: "transparent", color: textColor }}
		/>
	);

	return (
		<Box
			alignItems="center"
			bg={pageBg}
			display="flex"
			justifyContent="center"
			minH="100dvh"
			overflow="hidden"
			position="relative"
			px={{ base: 4, md: 10 }}
			py={{ base: 6, md: 10 }}
			w="full"
		>
			<Box
				bg="var(--rb-panel-accent)"
				borderRadius="full"
				filter="blur(90px)"
				h={{ base: "260px", md: "420px" }}
				left={{ base: "-15%", md: "-5%" }}
				opacity={blobOpacity}
				pointerEvents="none"
				position="absolute"
				top={{ base: "-10%", md: "-8%" }}
				w={{ base: "260px", md: "420px" }}
				css={{ animation: `${floatBlobA} 16s ease-in-out infinite` }}
			/>
			<Box
				bg="var(--rb-panel-accent-hover)"
				borderRadius="full"
				bottom={{ base: "-15%", md: "-10%" }}
				filter="blur(100px)"
				h={{ base: "280px", md: "460px" }}
				opacity={blobOpacity}
				pointerEvents="none"
				position="absolute"
				right={{ base: "-15%", md: "-6%" }}
				w={{ base: "280px", md: "460px" }}
				css={{ animation: `${floatBlobB} 20s ease-in-out infinite` }}
			/>
			<Box
				bg={borderColor}
				borderRadius="full"
				filter="blur(80px)"
				h={{ base: "220px", md: "320px" }}
				left="50%"
				opacity={blobOpacity}
				pointerEvents="none"
				position="absolute"
				top="55%"
				w={{ base: "220px", md: "320px" }}
				css={{ animation: `${floatBlobC} 14s ease-in-out infinite` }}
			/>
			<MotionBox
				animate={{ opacity: 1, y: 0 }}
				initial={{ opacity: 0, y: 28 }}
				position="relative"
				transition={{ duration: 0.5, ease: "easeOut" }}
				zIndex={1}
			>
				<VStack maxW="400px" spacing={6} w="full">
					<Box position="relative" w="full">
						<Box
							bg="var(--rb-panel-accent)"
							borderRadius="20px"
							filter="blur(40px)"
							h="70%"
							left="50%"
							opacity={0.35}
							pointerEvents="none"
							position="absolute"
							top="50%"
							w="90%"
							zIndex={-1}
							css={{ animation: `${glowPulse} 4s ease-in-out infinite` }}
						/>
						<Box
							bg={surfaceBg}
							borderColor={borderColor}
							borderRadius="16px"
							borderWidth="1px"
							boxShadow="0 30px 80px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.05)"
							p={{ base: 5, sm: 6 }}
							style={{
								transform: `perspective(1000px) rotateX(${tilt.rx}deg) rotateY(${tilt.ry}deg)`,
								transition: "transform 0.15s ease-out",
							}}
							w="full"
							onMouseLeave={handleCardMouseLeave}
							onMouseMove={handleCardMouseMove}
						>
							<HStack justifyContent="space-between" mb={7} spacing={3}>
								<HStack color={textColor} minW={0} spacing={3}>
									<Box
										alignItems="center"
										bg={elevatedBg}
										borderColor={borderColor}
										borderRadius="10px"
										borderWidth="1px"
										display="inline-flex"
										flexShrink={0}
										h={10}
										justifyContent="center"
										w={10}
										css={{
											animation: `${logoPulse} 2.6s ease-in-out infinite`,
										}}
									>
										<LogoIcon alt={t("menu")} src={logoUrl} />
									</Box>
									<Text fontSize="lg" fontWeight="800" noOfLines={1}>
										Next
									</Text>
								</HStack>
								<HStack flexShrink={0} spacing={2}>
									<Language triggerVariant="ghost" />
									<LoginThemeMenu />
								</HStack>
							</HStack>

							<VStack align="stretch" spacing={1} textAlign="center">
								<Text color={textColor} fontSize="lg" fontWeight="800">
									{step === "credentials"
										? t("login.welcome")
										: step === "otp"
											? t("login.twoFactorTitle")
											: t("login.setupTwoFactorTitle")}
								</Text>
								<Text color={mutedColor} fontSize="sm">
									{step === "credentials"
										? t("login.welcomeBack")
										: step === "otp"
											? t("login.twoFactorHint")
											: t("login.setupTwoFactorHint")}
								</Text>
							</VStack>

							<Box mt={6}>
								{step === "credentials" ? (
									<form onSubmit={handleSubmit(login, handleInvalid)}>
										<VStack spacing={4}>
											<LoginField
												autoComplete="username"
												dir={dir}
												errorMessage={
													errors.username?.message
														? t(errors.username.message as string)
														: undefined
												}
												icon={<User />}
												label={t("username")}
												placeholder={t("username")}
												registration={register("username")}
											/>
											<LoginField
												autoComplete="current-password"
												dir={dir}
												endElement={passwordToggle}
												errorMessage={
													errors.password?.message
														? t(errors.password.message as string)
														: undefined
												}
												icon={<Lock />}
												label={t("password")}
												placeholder={t("password")}
												registration={register("password")}
												type={showPassword ? "text" : "password"}
											/>

											{error && (
												<Alert
													borderRadius="8px"
													fontSize="sm"
													status="error"
													variant="left-accent"
													w="full"
												>
													<AlertIcon />
													<AlertDescription>{error}</AlertDescription>
												</Alert>
											)}

											<Button
												bg={accentColor}
												borderRadius="8px"
												color="white"
												h="44px"
												isDisabled={!canSubmit}
												isLoading={isSubmitting}
												leftIcon={<LoginIcon />}
												mt={1}
												type="submit"
												w="full"
												_hover={{ bg: "var(--rb-panel-accent-hover)" }}
												_active={{ transform: "translateY(1px)" }}
											>
												{t("login")}
											</Button>
										</VStack>
									</form>
								) : (
									<VStack spacing={4}>
										{step === "setup" && setup && (
											<>
												<Box bg="white" borderRadius="8px" p={3}>
													<QRCodeCanvas value={setup.uri} size={180} />
												</Box>
												<Text
													color={mutedColor}
													fontFamily="mono"
													fontSize="xs"
													wordBreak="break-all"
												>
													{setup.secret}
												</Text>
											</>
										)}
										<FormControl>
											<FormLabel>{t("login.authenticationCode")}</FormLabel>
											<CInput
												autoComplete="one-time-code"
												inputMode="numeric"
												maxLength={6}
												textAlign="center"
												value={otp}
												onChange={(event) =>
													setOTP(event.target.value.replace(/\D/g, ""))
												}
											/>
										</FormControl>
										{error && (
											<Alert
												borderRadius="8px"
												fontSize="sm"
												status="error"
												variant="left-accent"
												w="full"
											>
												<AlertIcon />
												<AlertDescription>{error}</AlertDescription>
											</Alert>
										)}
										<Button
											bg={accentColor}
											color="white"
											h="44px"
											isDisabled={otp.length !== 6}
											isLoading={challengeLoading}
											onClick={step === "otp" ? submitOTP : confirmSetup}
											w="full"
										>
											{t("continue")}
										</Button>
										<Button onClick={cancelChallenge} variant="ghost" w="full">
											{t("back")}
										</Button>
									</VStack>
								)}
							</Box>
						</Box>
					</Box>
				</VStack>
			</MotionBox>
		</Box>
	);
};

export default Login;
