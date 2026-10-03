import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { Participant, PresenceParticipant } from "../types";
import { ParticipantsPanel } from "./ParticipantsPanel";

const owner = {
  id: "owner",
  displayName: "Алиса",
  role: "owner",
  status: "joined",
  admissionState: "admitted",
} as Participant;
const colleague = {
  id: "colleague",
  displayName: "Борис Волков",
  role: "participant",
  status: "joined",
  admissionState: "admitted",
} as Participant;

/** Создаёт серверное подтверждение присутствия для панели участников.
 * @args person — членство участника; changes — отличия его текущего подключения и допуска.
 * @return Снимок присутствия с одним физическим подключением по умолчанию.
 */
function present(
  person: Participant,
  changes: Partial<PresenceParticipant> = {},
): PresenceParticipant {
  return {
    ...person,
    online: true,
    connections: 1,
    connectionIds: [`connection-${person.id}`],
    ...changes,
  };
}

it("не показывает offline и отсутствующее присутствие ни в списке, ни в действиях модерации", () => {
  const unknown = { ...colleague, id: "unknown", displayName: "Вера" };
  render(
    <ParticipantsPanel
      participants={[owner, colleague, unknown]}
      membership={owner}
      presence={[
        present(owner),
        present(colleague, {
          online: false,
          connections: 0,
          connectionIds: [],
        }),
      ]}
      loading={false}
      busy={false}
      onModerate={vi.fn()}
    />,
  );
  expect(screen.getByText("Алиса (вы)")).toBeInTheDocument();
  expect(screen.queryByText("Борис Волков")).toBeNull();
  expect(screen.queryByText("Вера")).toBeNull();
  expect(screen.queryByLabelText("Управление: Борис Волков")).toBeNull();
  expect(screen.queryByText(/Не в сети/)).toBeNull();
});

it("не добавляет себя и не предлагает модерацию до авторитетного снимка комнаты", () => {
  render(
    <ParticipantsPanel
      participants={[owner, colleague]}
      membership={owner}
      loading={false}
      busy={false}
      onModerate={vi.fn()}
    />,
  );
  expect(screen.queryByText("Алиса (вы)")).toBeNull();
  expect(screen.queryByText("Борис Волков")).toBeNull();
  expect(screen.queryByRole("button", { name: "Исключить" })).toBeNull();
  expect(screen.getByText("Участники не найдены.")).toBeInTheDocument();
});

it("удаляет строку и модерацию при offline-снимке, сохраняя участника при закрытии лишь одной вкладки", () => {
  const props = {
    participants: [owner, colleague],
    membership: owner,
    loading: false,
    busy: false,
    onModerate: vi.fn(),
  };
  const view = render(
    <ParticipantsPanel
      {...props}
      presence={[
        present(owner),
        present(colleague, {
          connections: 2,
          connectionIds: ["tab-one", "tab-two"],
        }),
      ]}
    />,
  );
  expect(screen.getAllByText("Борис Волков")).toHaveLength(1);
  expect(screen.getByLabelText("Управление: Борис Волков")).toBeInTheDocument();
  view.rerender(
    <ParticipantsPanel
      {...props}
      presence={[
        present(owner),
        present(colleague, { connections: 1, connectionIds: ["tab-two"] }),
      ]}
    />,
  );
  expect(screen.getAllByText("Борис Волков")).toHaveLength(1);
  view.rerender(
    <ParticipantsPanel
      {...props}
      presence={[
        present(owner),
        present(colleague, {
          online: false,
          connections: 0,
          connectionIds: [],
        }),
      ]}
    />,
  );
  expect(screen.queryByText("Борис Волков")).toBeNull();
  expect(screen.queryByLabelText("Управление: Борис Волков")).toBeNull();
});

it("показывает подключённых из следующих страниц и ищет только среди онлайн-участников", () => {
  const disconnected = {
    ...colleague,
    id: "disconnected",
    displayName: "Борис не в сети",
  };
  render(
    <ParticipantsPanel
      participants={[owner, disconnected]}
      membership={owner}
      presence={[
        present(owner),
        present(disconnected, { online: false }),
        present(colleague),
      ]}
      loading={false}
      busy={false}
      onModerate={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Поиск участника" }), {
    target: { value: "БОРИС" },
  });
  expect(screen.getByText("Борис Волков")).toBeInTheDocument();
  expect(screen.queryByText("Борис не в сети")).toBeNull();
  expect(screen.queryByText("Алиса (вы)")).toBeNull();
});

it("не смешивает ожидающих допуска и выбывших с присутствующими во встрече", () => {
  const waiting = {
    ...colleague,
    id: "waiting",
    displayName: "Ожидает допуска",
    status: "waiting",
    admissionState: "waiting",
  } as Participant;
  const left = {
    ...colleague,
    id: "left",
    displayName: "Уже вышел",
    status: "left",
  } as Participant;
  render(
    <ParticipantsPanel
      participants={[owner, waiting, left]}
      membership={owner}
      presence={[present(owner), present(waiting), present(left)]}
      loading={false}
      busy={false}
      onModerate={vi.fn()}
    />,
  );
  expect(screen.getByText("Алиса (вы)")).toBeInTheDocument();
  expect(screen.queryByText("Ожидает допуска")).toBeNull();
  expect(screen.queryByText("Уже вышел")).toBeNull();
});
