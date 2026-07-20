import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";

import "./style.css";

function formatBoolean(value: boolean, t: (key: string) => string) {
  return value
    ? t("global.k_Dashboard_Value_True")
    : t("global.k_Dashboard_Value_False");
}

function formatValue(value: string | number | null | undefined, t: (key: string) => string) {
  return value == null || value === ""
    ? t("global.k_Dashboard_Value_Empty")
    : String(value);
}

export default function Dashboard() {
  const navigate = useNavigate();
  const { t } = useTranslation("ui");
  const { signOut, user } = useAuth();
  useDocumentTitle(t("global.k_Dashboard_PageTitle"));

  async function handleLogout() {
    await signOut();
    navigate("/login", { replace: true });
  }

  return (
    <section className="dashboard-page">
      <header className="dashboard-page__header">
        <NexText variant="title">{t("global.k_Dashboard_Title")}</NexText>
        <NexButton type="button" variant="secondary" onClick={() => void handleLogout()}>
          {t("global.k_Dashboard_Button_Logout")}
        </NexButton>
      </header>

      {user ? (
        <div className="dashboard-user">
          <section className="dashboard-user__summary" aria-labelledby="dashboard-user-title">
            <NexText id="dashboard-user-title" variant="heading">
              {user.display_name}
            </NexText>
            <dl className="dashboard-user__fields">
              <div>
                <dt>{t("global.k_Dashboard_Label_UserId")}</dt>
                <dd>{user.id}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_Email")}</dt>
                <dd>{user.email}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_Status")}</dt>
                <dd>{user.status}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_Locale")}</dt>
                <dd>{formatValue(user.locale, t)}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_Timezone")}</dt>
                <dd>{formatValue(user.timezone, t)}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_Protected")}</dt>
                <dd>{formatBoolean(user.is_protected, t)}</dd>
              </div>
            </dl>
          </section>

          <section className="dashboard-user__section" aria-labelledby="dashboard-security-title">
            <NexText id="dashboard-security-title" variant="subheading">
              {t("global.k_Dashboard_Title_Security")}
            </NexText>
            <dl className="dashboard-user__fields">
              <div>
                <dt>{t("global.k_Dashboard_Label_MustChangePassword")}</dt>
                <dd>{formatBoolean(user.must_change_password, t)}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_FailedLoginCount")}</dt>
                <dd>{user.failed_login_count}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_LockedUntil")}</dt>
                <dd>{formatValue(user.locked_until, t)}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_LastLoginAt")}</dt>
                <dd>{formatValue(user.last_login_at, t)}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_PasswordChangedAt")}</dt>
                <dd>{formatValue(user.password_changed_at, t)}</dd>
              </div>
            </dl>
          </section>

          <section className="dashboard-user__section" aria-labelledby="dashboard-employee-title">
            <NexText id="dashboard-employee-title" variant="subheading">
              {t("global.k_Dashboard_Title_EmployeeProfile")}
            </NexText>
            {user.employee_profile ? (
              <dl className="dashboard-user__fields">
                <div>
                  <dt>{t("global.k_Dashboard_Label_EmployeeId")}</dt>
                  <dd>{user.employee_profile.id}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_EmployeeNumber")}</dt>
                  <dd>{user.employee_profile.employee_number}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_LegalName")}</dt>
                  <dd>{formatValue(user.employee_profile.legal_name, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_PreferredName")}</dt>
                  <dd>{formatValue(user.employee_profile.preferred_name, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_WorkEmail")}</dt>
                  <dd>{formatValue(user.employee_profile.work_email, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_WorkPhone")}</dt>
                  <dd>{formatValue(user.employee_profile.work_phone, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_JobTitle")}</dt>
                  <dd>{formatValue(user.employee_profile.job_title, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_EmploymentStatus")}</dt>
                  <dd>{user.employee_profile.employment_status}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_HireDate")}</dt>
                  <dd>{formatValue(user.employee_profile.hire_date, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_TerminationDate")}</dt>
                  <dd>{formatValue(user.employee_profile.termination_date, t)}</dd>
                </div>
                <div>
                  <dt>{t("global.k_Dashboard_Label_ManagerEmployeeId")}</dt>
                  <dd>{formatValue(user.employee_profile.manager_employee_id, t)}</dd>
                </div>
              </dl>
            ) : (
              <NexText color="muted">
                {t("global.k_Dashboard_Content_NoEmployeeProfile")}
              </NexText>
            )}
          </section>

          <section className="dashboard-user__section" aria-labelledby="dashboard-roles-title">
            <NexText id="dashboard-roles-title" variant="subheading">
              {t("global.k_Dashboard_Title_Roles")}
            </NexText>
            {user.roles.length ? (
              <div className="dashboard-user__items">
                {user.roles.map((role) => (
                  <dl className="dashboard-user__item" key={role.id}>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_RoleName")}</dt>
                      <dd>{role.title}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_RoleDescription")}</dt>
                      <dd>{formatValue(role.description, t)}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_SystemRole")}</dt>
                      <dd>{formatBoolean(role.is_system, t)}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_GrantsAllPermissions")}</dt>
                      <dd>{formatBoolean(role.grants_all_permissions, t)}</dd>
                    </div>
                  </dl>
                ))}
              </div>
            ) : (
              <NexText color="muted">{t("global.k_Dashboard_Content_NoRoles")}</NexText>
            )}
          </section>

          <section className="dashboard-user__section" aria-labelledby="dashboard-groups-title">
            <NexText id="dashboard-groups-title" variant="subheading">
              {t("global.k_Dashboard_Title_Groups")}
            </NexText>
            {user.groups.length ? (
              <div className="dashboard-user__items">
                {user.groups.map((group) => (
                  <dl className="dashboard-user__item" key={group.id}>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_GroupName")}</dt>
                      <dd>{group.name}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_GroupType")}</dt>
                      <dd>{group.type}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_GroupStatus")}</dt>
                      <dd>{group.status}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_GroupTitle")}</dt>
                      <dd>{formatValue(group.title, t)}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_JoinedAt")}</dt>
                      <dd>{formatValue(group.joined_at, t)}</dd>
                    </div>
                    <div>
                      <dt>{t("global.k_Dashboard_Label_LeftAt")}</dt>
                      <dd>{formatValue(group.left_at, t)}</dd>
                    </div>
                  </dl>
                ))}
              </div>
            ) : (
              <NexText color="muted">{t("global.k_Dashboard_Content_NoGroups")}</NexText>
            )}
          </section>

          <section className="dashboard-user__section" aria-labelledby="dashboard-lifecycle-title">
            <NexText id="dashboard-lifecycle-title" variant="subheading">
              {t("global.k_Dashboard_Title_Lifecycle")}
            </NexText>
            <dl className="dashboard-user__fields">
              <div>
                <dt>{t("global.k_Dashboard_Label_CreatedAt")}</dt>
                <dd>{user.created_at}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_UpdatedAt")}</dt>
                <dd>{user.updated_at}</dd>
              </div>
              <div>
                <dt>{t("global.k_Dashboard_Label_DeletedAt")}</dt>
                <dd>{formatValue(user.deleted_at, t)}</dd>
              </div>
            </dl>
          </section>
        </div>
      ) : (
        <NexText color="muted">
          {t("global.k_Dashboard_Content_NoAuthenticatedUser")}
        </NexText>
      )}
    </section>
  );
}
