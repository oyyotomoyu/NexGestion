import { useTranslation } from "react-i18next";

import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";

export default function Dashboard() {
  const { t } = useTranslation("ui");
  useDocumentTitle(t("global.k_Dashboard_PageTitle"));

  return (
    <section>
      <NexText variant="title">{t("global.k_Dashboard_Title")}</NexText>
      <NexText>{t("global.k_Dashboard_Content_Ready")}</NexText>
    </section>
  );
}
