/** Постановочное изображение экрана видеоконференции; не использует данные участников приложения. */
export function AuthTeamVisual() {
  return (
    <div className="auth-team-photo" aria-hidden="true">
      <img
        src="/media/auth-videoconference-v3.webp"
        width="1536"
        height="1024"
        alt=""
        decoding="async"
        fetchPriority="high"
      />
    </div>
  );
}
