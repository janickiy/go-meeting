import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, LogIn, Video } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { Button, ErrorNotice, Loading, StatusBadge } from "../components/ui";
import { formatDate } from "../utils";

/**
 * InvitePage показывает сведения приглашения и обрабатывает авторизованный вход или включение в будущую встречу.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function InvitePage() {
  const { code = "" } = useParams();
  const { user } = useAuth();
  const navigate = useNavigate();
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["invite", user?.id, code],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.invite(code, signal).
     */
    queryFn: ({ signal }) => api.invite(code, signal),
  });
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленное значение: api.joinInvite(code).
     */
    mutationFn: () => api.joinInvite(code),
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     * @args
     *   - объект параметров: item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
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
        <Button
          busy={mutation.isPending}
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: mutation.mutate().
             */ () => mutation.mutate()
          }
        >
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
