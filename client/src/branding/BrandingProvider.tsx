import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { defaultTheme, NexColor, type NexColors, type NexColorTheme } from "@/components/NexColor";
import { getBranding, type Branding, type BrandingImageName } from "@/requests/systemConfig";
import defaultFaviconUrl from "@odm/img/favicon.ico?url";
import defaultLanguageIcon from "@odm/img/language-icon.svg";
import defaultLoginLogo from "@odm/img/login-logo.svg";

// The odm/ files stay the build-time defaults. System Config stores runtime
// overrides on the server (docs/System/system-config.md Section 4), which are
// layered on top here at startup, so a saved change applies on the next page
// load without rebuilding the client.

const defaultImages: Record<BrandingImageName, string> = {
  "login-logo.svg": defaultLoginLogo,
  "language-icon.svg": defaultLanguageIcon,
  "favicon.ico": defaultFaviconUrl,
};

interface BrandingContextValue {
  branding: Branding;
  /** Effective image URL: the server override when present, else the bundled default. */
  image: (name: BrandingImageName) => string;
  defaultImage: (name: BrandingImageName) => string;
  /** Re-reads overrides after an edit so it applies without a reload. */
  refresh: () => Promise<void>;
  /** Temporarily applies unsaved colors for a live preview; null ends it. */
  previewColors: (colors: Partial<NexColors> | null) => void;
}

const emptyBranding: Branding = { colors: {}, images: {}, updated_at: null };
const BrandingContext = createContext<BrandingContextValue | null>(null);

function mergeTheme(overrides: Record<string, string>): NexColorTheme {
  const colors = { ...defaultTheme.colors };
  (Object.keys(colors) as (keyof NexColors)[]).forEach((token) => {
    if (overrides[token]) colors[token] = overrides[token];
  });
  return { ...defaultTheme, colors };
}

function applyFavicon(href: string) {
  let link = document.querySelector<HTMLLinkElement>("link[rel='icon']");
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    link.type = "image/x-icon";
    link.sizes = "32x32";
    document.head.append(link);
  }
  if (link.href !== href) link.href = href;
}

export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<Branding>(emptyBranding);
  const [preview, setPreview] = useState<Partial<NexColors> | null>(null);

  const refresh = useCallback(async () => {
    try {
      setBranding(await getBranding());
    } catch {
      // Unreachable server or no overrides endpoint: keep bundled defaults.
      setBranding(emptyBranding);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const image = useCallback(
    (name: BrandingImageName) => branding.images[name]?.url ?? defaultImages[name],
    [branding],
  );

  useEffect(() => {
    applyFavicon(image("favicon.ico"));
  }, [image]);

  const theme = useMemo(
    () => mergeTheme({ ...branding.colors, ...(preview as Record<string, string> | null) }),
    [branding.colors, preview],
  );

  const value = useMemo<BrandingContextValue>(
    () => ({
      branding,
      image,
      defaultImage: (name) => defaultImages[name],
      refresh,
      previewColors: setPreview,
    }),
    [branding, image, refresh],
  );

  return (
    <BrandingContext.Provider value={value}>
      <NexColor theme={theme}>{children}</NexColor>
    </BrandingContext.Provider>
  );
}

export function useBranding() {
  const context = useContext(BrandingContext);
  if (!context) {
    throw new Error("useBranding must be used inside BrandingProvider");
  }
  return context;
}
