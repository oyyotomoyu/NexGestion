import { useTranslation } from "react-i18next";

import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";

export default function Settings() {
  const { t } = useTranslation("ui");
  useDocumentTitle(t("global.k_Settings_PageTitle"));

  return (
    <section>
      <NexText variant="title">{t("global.k_Settings_Title")}</NexText>
      <NexText>{t("global.k_Settings_Content_Planned")}</NexText>
    </section>
  );
}
