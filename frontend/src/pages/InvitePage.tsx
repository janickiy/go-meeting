import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, LogIn, Video } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { Button, ErrorNotice, Loading, StatusBadge } from "../components/ui";
import { formatDate } from "../utils";

export function InvitePage() {
  const { code = "" } = useParams();
  const { user } = useAuth();
  const navigate = useNavigate();
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["invite", user?.id, code],
    queryFn: ({ signal }) => api.invite(code, signal),
  });
  const mutation = useMutation({
    mutationFn: () => api.joinInvite(code),
    onSuccess: ({ item }) => {
      void client.invalidateQueries({ queryKey: ["conferences"] });
      navigate(`/conferences/${item.conferenceId}`, { replace: true });
    },
  });
  if (query.isPending) return <Loading />;
  if (query.isError || !query.data)
    return (
      <div className="content-card">
        <h1>Приглашение недоступно</h1>
        <ErrorNotice error={query.error} />
        <Link className="text-link" to="/app">
          <ArrowLeft size={16} />В мой кабинет
        </Link>
      </div>
    );
  const conference = query.data.item;
  const closed =
    conference.status === "finished" || conference.status === "cancelled";
  return (
    <section className="content-card invite-preview">
      <span className="meeting-card-symbol">
        <Video size={36} />
      </span>
      <span className="eyebrow">ВАС ПРИГЛАСИЛИ НА ВСТРЕЧУ</span>
      <h1>{conference.title}</h1>
      <StatusBadge status={conference.status} />
      {conference.scheduledAt && (
        <p>Начало: {formatDate(conference.scheduledAt)}</p>
      )}
      {conference.waitingRoomEnabled && (
        <p className="field-hint">
          Во встрече включён зал ожидания. Организатор подтвердит вход после
          начала.
        </p>
      )}
      <p>
        {closed
          ? "Организатор уже закрыл эту конференцию."
          : "Нажмите кнопку ниже, чтобы присоединиться с вашим аккаунтом Meet."}
      </p>
      <ErrorNotice error={mutation.error} />
      {!closed && (
        <Button busy={mutation.isPending} onClick={() => mutation.mutate()}>
          <LogIn size={18} />
          {conference.status === "scheduled"
            ? "Добавить в мои встречи"
            : "Присоединиться к конференции"}
        </Button>
      )}
      <Link className="text-link" to="/app">
        Вернуться в мой кабинет
      </Link>
    </section>
  );
}
