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

export const LoginPage = () => <LoginView />;
