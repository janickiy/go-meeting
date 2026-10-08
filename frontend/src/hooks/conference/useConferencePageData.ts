import { useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { useAuth } from "../../auth";
import { isAdmitted } from "../../collaboration";
import { onlineParticipants } from "../../presence";
import { useConference, useMembership, useParticipants } from "../../queries";
import { useRealtime } from "../../realtime";
import { useCapabilities } from "../../useCapabilities";

/**
 * Собирает серверные данные страницы и владеет единственным realtime-подключением встречи.
 * Панели получают этот же live-объект: переключение вкладок не создаёт новые сокеты.
 * Запросы истории и записей сохраняют условия доступа и прежние интервалы обновления.
 * @args id — идентификатор встречи из маршрута.
 * @return Запросы, членство, присутствие и вычисленные разрешения текущего представления.
 */
export function useConferencePageData(id: string) {
  const { user } = useAuth();
  const query = useConference(id);
  const self = useMembership(id);
  const conference = query.data?.item;
  const membership = self.data || undefined;
  const admitted = isAdmitted(membership);
  const participants = useParticipants(id, admitted);
  const people = participants.data?.pages.flatMap((page) => page.items) || [];
  const closed = ["finished", "cancelled"].includes(conference?.status || "");
  const live = useRealtime(
    id,
    admitted && membership?.status === "joined" && !closed,
  );
  const onlinePeople = onlineParticipants(people, live.state?.participants);
  const displayedPeople = closed ? people : onlinePeople;
  const capabilities = useCapabilities();
  const features =
    capabilities.isSuccess && !capabilities.isError
      ? capabilities.data.capabilities
      : undefined;
  const captionsEnabled = features?.liveCaptions === true;
  const analyticsEnabled = features?.meetingAnalytics === true;
  const activeMeeting =
    admitted &&
    membership?.status === "joined" &&
    conference?.status === "active";
  const recordingAccess = Boolean(user && !user.guestConferenceId);
  const recordingStatus = useQuery({
    queryKey: ["recordings", id],
    queryFn: ({ signal }) => api.recordings(id, signal),
    enabled: activeMeeting,
    refetchInterval: activeMeeting ? 3000 : false,
  });
  const history = useQuery({
    queryKey: ["history", user?.id, id],
    queryFn: ({ signal }) => api.history(id, signal),
    enabled: admitted && closed,
  });
  const owner = conference?.ownerId === user?.id;
  const canInvite = Boolean(
    user &&
    !user.guestConferenceId &&
    !closed &&
    (owner || (activeMeeting && membership?.role === "co_host")),
  );
  const activeRecording = !recordingStatus.isError
    ? recordingStatus.data?.items.find(
        (item) =>
          item.conferenceId === conference?.id &&
          ["starting", "recording", "degraded", "stopping"].includes(
            item.status,
          ),
      )
    : undefined;

  return {
    user,
    query,
    self,
    conference,
    membership,
    admitted,
    participants,
    people,
    closed,
    live,
    onlinePeople,
    displayedPeople,
    capabilities,
    captionsEnabled,
    analyticsEnabled,
    activeMeeting,
    recordingAccess,
    recordingStatus,
    activeRecording,
    history,
    owner,
    canInvite,
  };
}

/** Набор данных и запросов, общий для представлений одной страницы встречи. */
export type ConferencePageData = ReturnType<typeof useConferencePageData>;
