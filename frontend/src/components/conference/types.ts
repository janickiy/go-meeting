import type { Conference, Participant } from "../../types";
import type { ConferencePageData } from "../../hooks/conference/useConferencePageData";
import type { ConferenceCommands } from "../../hooks/conference/useConferenceCommands";
import type { ConferenceRoomControls } from "../../hooks/conference/useConferenceRoomControls";

/** Общие данные представления встречи: состояние и команды создаются только в корневой странице.
 * @params id — маршрут; data — загруженная встреча и запросы; commands — серверные команды;
 * controls — сохраняемое состояние панелей и диалогов.
 */
export interface ConferenceViewProps {
  id: string;
  data: ConferencePageData & { conference: Conference };
  commands: ConferenceCommands;
  controls: ConferenceRoomControls;
}

/** Активная комната получает только подтверждённое собственное членство. */
export interface ActiveConferenceViewProps extends ConferenceViewProps {
  data: ConferenceViewProps["data"] & { membership: Participant };
}
