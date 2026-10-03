import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { Participant } from "../types";
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

it("передаёт модерацию серверу и не меняет разрешения до подтверждённого состояния", () => {
  const onModerate = vi.fn();
  render(
    <ParticipantsPanel
      participants={[owner, colleague]}
      membership={owner}
      loading={false}
      busy={false}
      onModerate={onModerate}
    />,
  );
  expect(screen.queryByLabelText("Управление: Алиса")).toBeNull();
  const controls = screen.getByLabelText("Управление: Борис Волков");
  fireEvent.click(
    within(controls).getByRole("button", { name: "Отключить микрофон" }),
  );
  expect(onModerate).toHaveBeenCalledExactlyOnceWith(colleague.id, {
    action: "mute",
    blocked: true,
  });
  expect(
    within(controls).queryByRole("button", { name: "Разрешить микрофон" }),
  ).toBeNull();
});

it("не предлагает соорганизатору чужую роль или запрет камеры, поиск работает по реальному списку", () => {
  const cohost = { ...owner, role: "co_host" } as Participant;
  render(
    <ParticipantsPanel
      participants={[cohost, colleague]}
      membership={cohost}
      loading={false}
      busy={false}
      onModerate={vi.fn()}
    />,
  );
  expect(screen.queryByRole("button", { name: "Отключить видео" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: "Назначить соорганизатором" }),
  ).toBeNull();
  fireEvent.change(screen.getByRole("textbox", { name: "Поиск участника" }), {
    target: { value: "Борис" },
  });
  expect(screen.getByText("Борис Волков")).toBeInTheDocument();
  expect(screen.queryByText("Алиса (вы)")).toBeNull();
});
