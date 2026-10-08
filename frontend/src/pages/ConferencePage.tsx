import { Link, useParams } from "react-router";
import { ArrowLeft } from "lucide-react";
import { ErrorNotice, Loading } from "../components/ui";
import { ActiveConferenceRoom } from "../components/conference/ActiveConferenceRoom";
import { ConferenceOverview } from "../components/conference/ConferenceOverview";
import { useConferencePageData } from "../hooks/conference/useConferencePageData";
import { useConferenceCommands } from "../hooks/conference/useConferenceCommands";
import { useConferenceRoomControls } from "../hooks/conference/useConferenceRoomControls";
import "./conference.css";

/**
 * Координирует жизненный цикл страницы встречи и выбирает активное или обзорное представление.
 * Общие запросы, команды и единственное realtime-подключение создаются до выбора представления,
 * поэтому смена статуса и переключение панелей не создают вторую медиасессию.
 * @return Загрузка, ошибка доступа либо представление текущего состояния встречи.
 */
export function ConferencePage() {
  const { id = "" } = useParams();
  const data = useConferencePageData(id);
  const controls = useConferenceRoomControls(
    id,
    data.activeMeeting,
    data.captionsEnabled,
  );
  const commands = useConferenceCommands(id, () => controls.setConfirm(null));

  if (data.query.isPending) return <Loading />;
  if (data.query.isError || !data.conference)
    return (
      <div className="content-card">
        <ErrorNotice error={data.query.error || new Error()} />
        <Link
          to={data.user?.guestConferenceId ? "/" : "/conferences"}
          className="text-link"
        >
          <ArrowLeft size={16} />К моим конференциям
        </Link>
      </div>
    );

  const resolvedData = { ...data, conference: data.conference };
  if (data.activeMeeting && data.membership)
    return (
      <ActiveConferenceRoom
        id={id}
        data={{ ...resolvedData, membership: data.membership }}
        commands={commands}
        controls={controls}
      />
    );
  return (
    <ConferenceOverview
      id={id}
      data={resolvedData}
      commands={commands}
      controls={controls}
    />
  );
}
