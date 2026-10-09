import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { Brand, ErrorNotice, Loading } from "../components/ui";
import { PreJoinPage } from "./PreJoinPage";

/** Показывает общедоступные сведения приглашения; вход создаёт отдельную гостевую сессию встречи. */
export function InvitePage() {
  const { code = "" } = useParams();
  const valid = /^[A-Za-z0-9_-]{32}$/.test(code);
  const query = useQuery({
    queryKey: ["public-invite", code],
    queryFn: ({ signal }) => api.invite(code, signal),
    enabled: valid,
    retry: false,
  });
  if (valid && query.isPending)
    return (
      <main className="invite-entry-page">
        <div className="invite-entry-brand">
          <Brand />
        </div>
        <Loading />
      </main>
    );
  if (!valid || query.isError || !query.data)
    return (
      <main className="invite-entry-page">
        <div className="invite-entry-brand">
          <Brand />
        </div>
        <section className="invite-entry-error">
          <h1>Приглашение недоступно</h1>
          <p>Проверьте ссылку или попросите организатора прислать новую.</p>
          <ErrorNotice error={query.error} />
          <Link to="/">На главную</Link>
        </section>
      </main>
    );
  return <PreJoinPage invitation={{ code, meeting: query.data.item }} />;
}
