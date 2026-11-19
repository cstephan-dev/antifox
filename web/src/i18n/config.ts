import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import enUS from "./locales/en-US.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";

// 支持的三种语言。label 用各自母语名（永不翻译），供语言切换器展示。
export const LANGS = [
  { code: "zh-CN", label: "简体中文" },
  { code: "zh-TW", label: "繁體中文" },
  { code: "en-US", label: "English" },
] as const;

export type LangCode = (typeof LANGS)[number]["code"];

export const LANG_STORAGE_KEY = "antifox-lang";

export function isLangCode(v: string | null | undefined): v is LangCode {
  return !!v && LANGS.some((l) => l.code === v);
}

// Default UI language for users who have not picked one yet. Build-time setting (static export):
// NEXT_PUBLIC_DEFAULT_LANG=en-US npm run build:static. Unset or unknown → zh-CN.
const ENV_DEFAULT_LANG = process.env.NEXT_PUBLIC_DEFAULT_LANG;
export const DEFAULT_LANG: LangCode = isLangCode(ENV_DEFAULT_LANG) ? ENV_DEFAULT_LANG : "zh-CN";

// 资源直接打包（非异步加载）——静态导出友好、无 Suspense、SSR/首帧一致。
if (!i18n.isInitialized) {
  void i18n.use(initReactI18next).init({
    resources: {
      "zh-CN": { translation: zhCN },
      "zh-TW": { translation: zhTW },
      "en-US": { translation: enUS },
    },
    lng: DEFAULT_LANG,
    // zh-TW: a key it lacks falls back to Simplified Chinese (closer than the default language)
    fallbackLng: { "zh-TW": ["zh-CN", DEFAULT_LANG], default: [DEFAULT_LANG] },
    interpolation: { escapeValue: false },
    returnNull: false,
  });
}

export default i18n;
