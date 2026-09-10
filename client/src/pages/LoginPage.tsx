import { useState, type FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
    faBell,
    faFaceSmile,
    faEye,
    faEyeSlash,
    faQrcode,
    faShieldHalved,
    faUserShield,
} from "@fortawesome/free-solid-svg-icons";
import { Input } from "@/shared/ui";
import logoUrl from "@/assets/logo.png";
import { toLoginError } from "@/features/auth/api/auth";
import { useAuth } from "@/features/auth/useAuth";

interface StatusMessage {
    tone: "success" | "error";
    text: string;
}

export const LoginPage = () => {
    const { status: authStatus, user, error: authError, login, logout } =
        useAuth();
    const [showPassword, setShowPassword] = useState(false);
    const [cccd, setCccd] = useState("");
    const [password, setPassword] = useState("");
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [message, setMessage] = useState<StatusMessage | null>(null);

    const isBusy = isSubmitting || authStatus === "restoring";
    const formError =
        message?.tone === "error" ? message.text : authError;
    const successMessage =
        message?.tone === "success" ? message.text : null;

    const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (isSubmitting) return;

        setIsSubmitting(true);
        setMessage(null);
        try {
            await login({ cccdNumber: cccd, password });
            setPassword("");
            setMessage({ tone: "success", text: "Đăng nhập thành công." });
        } catch (submitError) {
            setMessage({
                tone: "error",
                text: toLoginError(submitError),
            });
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <div className="min-h-screen w-full bg-background">
            <main className="relative z-10 flex flex-col min-h-screen w-full px-4 py-3 sm:px-6 sm:py-4 md:px-8 md:py-5">
                <div className="w-full max-w-sm mx-auto sm:max-w-md md:max-w-lg lg:max-w-xl flex-1 flex flex-col">
                    {/* Header */}
                    <section className="w-full flex items-center justify-between pb-4 sm:pb-5 md:pb-6">
                        <div className="flex items-center space-x-2.5">
                            <img
                                alt="AnomalyBank"
                                className="h-12 sm:h-14 md:h-16 w-auto max-w-[265px] object-contain"
                                src={logoUrl}
                            />
                        </div>
                        <div className="flex items-center space-x-2.5">
                            <button
                                aria-label="Ngôn ngữ Tiếng Việt"
                                className="w-9 h-9 sm:w-10 sm:h-10 rounded-full bg-surface-container-lowest/80 backdrop-blur-md shadow-sm flex items-center justify-center p-1 transition-transform active:scale-95"
                                type="button"
                            >
                                <div className="w-6 h-6 sm:w-7 sm:h-7 rounded-full bg-[#da251d] relative flex items-center justify-center overflow-hidden">
                                    <svg
                                        className="w-3.5 h-3.5 sm:w-4 sm:h-4 fill-[#fffe00]"
                                        viewBox="0 0 100 100"
                                    >
                                        <polygon points="50,15 61,43 91,43 66,61 76,89 50,71 24,89 34,61 9,43 39,43" />
                                    </svg>
                                </div>
                            </button>
                            <button
                                aria-label="Thông báo hệ thống"
                                className="w-10 h-10 sm:w-11 sm:h-11 md:w-12 md:h-12 rounded-full bg-surface-container-lowest/80 backdrop-blur-md shadow-sm flex items-center justify-center text-on-surface relative transition-transform active:scale-95"
                                type="button"
                            >
                                <FontAwesomeIcon
                                    icon={faBell}
                                    className="text-lg sm:text-xl md:text-2xl"
                                    style={{ color: "#9e5e9e" }}
                                />
                                <span
                                    className="absolute top-1.5 right-1.5 sm:top-2 sm:right-2 w-2 h-2 sm:w-2.5 sm:h-2.5 rounded-full ring-2 ring-surface-container-lowest"
                                    style={{ backgroundColor: "#b582b5" }}
                                />
                            </button>
                        </div>
                    </section>

                    {/* Login Card */}
                    <div className="w-full my-auto">
                        <form
                            onSubmit={(event) => {
                                void handleSubmit(event);
                            }}
                            className="w-full rounded-xl sm:rounded-2xl bg-surface-container-low/60 backdrop-blur-xl shadow-2xl shadow-indigo-900/10 flex flex-col overflow-hidden relative"
                        >
                            <div className="absolute inset-x-0 top-0 h-20 sm:h-24 bg-gradient-to-b from-white/60 to-transparent pointer-events-none" />

                            <div className="p-4 sm:p-5 md:p-6 flex flex-col space-y-3.5 sm:space-y-4 relative z-10">
                                {/* Greeting */}
                                <div className="pt-1 flex items-center justify-between">
                                    <div className="flex flex-col">
                                        <div className="flex items-center space-x-1 text-on-surface-variant">
                                            <span className="text-base sm:text-lg md:text-title-md font-medium">
                                                Xin chào,
                                            </span>
                                            <FontAwesomeIcon
                                                icon={faUserShield}
                                                className="text-sm sm:text-base"
                                                style={{ color: "#b582b5" }}
                                            />
                                        </div>
                                        <p className="text-xs sm:text-label-sm text-on-surface-variant/70 mt-0.5">
                                            Vui lòng xác thực tài khoản định
                                            danh
                                        </p>
                                    </div>
                                    <button
                                        aria-label="Đăng nhập bằng sinh trắc học"
                                        className="w-10 h-10 sm:w-11 sm:h-11 rounded-full bg-surface-container-high/80 hover:bg-surface-variant flex items-center justify-center transition-all active:scale-90 shadow-xs ring-2 ring-surface-container-lowest"
                                        style={{
                                            backgroundColor:
                                                "rgba(181,130,181,0.22)",
                                            color: "#8f4c8f",
                                        }}
                                        type="button"
                                    >
                                        <FontAwesomeIcon
                                            icon={faFaceSmile}
                                            className="text-lg sm:text-xl md:text-2xl"
                                        />
                                    </button>
                                </div>

                                {/* CCCD Input */}
                                <Input
                                    label="Số Căn cước Công dân"
                                    type="text"
                                    name="cccd-number"
                                    autoComplete="username"
                                    inputMode="numeric"
                                    maxLength={14}
                                    placeholder="Nhập số CCCD"
                                    value={cccd}
                                    disabled={isBusy}
                                    onChange={(event) =>
                                        setCccd(event.target.value)
                                    }
                                />

                                {/* Password Input */}
                                <div className="flex flex-col gap-1">
                                    <div className="flex items-center justify-between">
                                        <label className="text-label-md font-semibold text-on-surface-variant">
                                            Mật khẩu
                                        </label>
                                        <a
                                            className="text-xs sm:text-label-sm text-secondary hover:underline cursor-pointer"
                                            href="#"
                                            style={{ color: "#9e5e9e" }}
                                        >
                                            Quên mật khẩu?
                                        </a>
                                    </div>
                                    <div className="flex items-center bg-surface-container-lowest/90 px-3.5 py-2.5 sm:px-4 sm:py-3 shadow-xs rounded-xl sm:rounded-2xl">
                                        <input
                                            className="w-full bg-transparent text-base sm:text-title-lg font-bold tracking-tight text-on-surface focus:outline-none placeholder:text-on-surface-variant/50"
                                            placeholder="••••••••"
                                            name="password"
                                            autoComplete="current-password"
                                            maxLength={128}
                                            value={password}
                                            disabled={isBusy}
                                            onChange={(event) =>
                                                setPassword(event.target.value)
                                            }
                                            type={
                                                showPassword
                                                    ? "text"
                                                    : "password"
                                            }
                                        />
                                        <button
                                            aria-label="Ẩn/hiện mật khẩu"
                                            className="flex items-center justify-center text-on-surface-variant/70 hover:text-on-surface transition-colors ml-2"
                                            onClick={() =>
                                                setShowPassword(!showPassword)
                                            }
                                            type="button"
                                        >
                                            <FontAwesomeIcon
                                                icon={
                                                    showPassword
                                                        ? faEye
                                                        : faEyeSlash
                                                }
                                                className="text-base sm:text-lg"
                                            />
                                        </button>
                                    </div>
                                </div>
                                {(formError || successMessage) && (
                                    <p
                                        role={
                                            formError ? "alert" : "status"
                                        }
                                        className={`rounded-xl px-3 py-2 text-xs sm:text-label-md ${
                                            formError
                                                ? "bg-error-container text-on-error-container"
                                                : "bg-surface-container-high text-on-surface"
                                        }`}
                                    >
                                        {formError ?? successMessage}
                                    </p>
                                )}
                            </div>

                            {/* Login Button */}
                            <button
                                className="w-full py-3.5 sm:py-4 font-semibold text-base sm:text-title-md tracking-wide text-center flex items-center justify-center transition-all active:scale-[0.99] disabled:opacity-80 disabled:active:scale-100 disabled:cursor-not-allowed"
                                style={{
                                    background:
                                        "linear-gradient(135deg, #b582b5 0%, #8b5cf6 100%)",
                                    color: "#ffffff",
                                    boxShadow:
                                        "rgba(181,130,181,0.45) 0px 4px 18px -2px",
                                }}
                                type="submit"
                                disabled={isBusy}
                            >
                                {authStatus === "restoring"
                                    ? "Đang kiểm tra phiên..."
                                    : isSubmitting
                                      ? "Đang đăng nhập..."
                                      : "Đăng nhập"}
                            </button>

                            {/* Register Link */}
                            <div className="w-full py-3 sm:py-3.5 text-center flex items-center justify-center bg-surface-container-lowest/40">
                                {authStatus === "authenticated" && user ? (
                                    <p className="text-xs sm:text-label-md text-on-surface-variant">
                                        Đã đăng nhập:{" "}
                                        <span className="font-semibold text-on-surface">
                                            {user.username}
                                        </span>{" "}
                                        <button
                                            type="button"
                                            onClick={() => {
                                                setMessage(null);
                                                logout();
                                            }}
                                            className="ml-1 font-semibold hover:underline cursor-pointer"
                                            style={{ color: "#9e5e9e" }}
                                        >
                                            Đăng xuất
                                        </button>
                                    </p>
                                ) : (
                                    <a
                                        className="text-xs sm:text-label-md text-on-surface-variant hover:text-on-surface transition-colors cursor-pointer"
                                        href="#"
                                    >
                                        Chưa có tài khoản?{" "}
                                        <span
                                            className="font-semibold hover:underline"
                                            style={{ color: "#9e5e9e" }}
                                        >
                                            Mở tài khoản ngay
                                        </span>
                                    </a>
                                )}
                            </div>
                        </form>

                        {/* Bottom Quick Actions */}
                        <div className="w-full grid grid-cols-2 gap-3 sm:gap-4 pt-4 mt-auto">
                            <button
                                className="flex flex-col items-center justify-center py-2.5 sm:py-3 px-2.5 sm:px-3 rounded-xl sm:rounded-2xl bg-surface-container-lowest/70 backdrop-blur-md shadow-sm active:scale-95 transition-all group"
                                type="button"
                            >
                                <div
                                    className="w-7 h-7 sm:w-8 sm:h-8 rounded-full bg-surface-container-high flex items-center justify-center mb-1.5 shadow-xs group-hover:bg-primary-container group-hover:text-on-primary transition-colors"
                                    style={{
                                        backgroundColor:
                                            "rgba(181,130,181,0.25)",
                                        color: "#8f4c8f",
                                    }}
                                >
                                    <FontAwesomeIcon
                                        icon={faQrcode}
                                        className="text-xs sm:text-sm md:text-base"
                                    />
                                </div>
                                <span className="font-semibold text-xs sm:text-[13px] text-on-surface text-center tracking-tight">
                                    Quét VietQR
                                </span>
                                <span className="text-[10px] sm:text-[11px] text-on-surface-variant/80 text-center leading-tight mt-0.5">
                                    Thanh toán & Chuyển khoản
                                </span>
                            </button>
                            <button
                                className="flex flex-col items-center justify-center py-2.5 sm:py-3 px-2.5 sm:px-3 rounded-xl sm:rounded-2xl bg-surface-container-lowest/70 backdrop-blur-md shadow-sm active:scale-95 transition-all group"
                                type="button"
                            >
                                <div
                                    className="w-7 h-7 sm:w-8 sm:h-8 rounded-full bg-surface-container-high flex items-center justify-center mb-1.5 shadow-xs group-hover:bg-primary-container group-hover:text-on-primary transition-colors"
                                    style={{
                                        backgroundColor:
                                            "rgba(181,130,181,0.25)",
                                        color: "#8f4c8f",
                                    }}
                                >
                                    <FontAwesomeIcon
                                        icon={faShieldHalved}
                                        className="text-xs sm:text-sm md:text-base"
                                    />
                                </div>
                                <span className="font-semibold text-xs sm:text-[13px] text-on-surface text-center tracking-tight">
                                    Xác thực D-OTP
                                </span>
                                <span className="text-[10px] sm:text-[11px] text-on-surface-variant/80 text-center leading-tight mt-0.5">
                                    Mã bảo mật tức thì
                                </span>
                            </button>
                        </div>
                    </div>
                </div>
            </main>
        </div>
    );
};
