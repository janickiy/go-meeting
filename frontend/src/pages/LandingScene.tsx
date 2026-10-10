import { ArrowUpRight, MessageCircle, Mic, Phone, Video } from "lucide-react";

/** Декоративная иллюстрация: элементы встречи не являются настоящими контролами. */
export function LandingScene() {
  return (
    <figure
      className="home-visual"
      aria-label="Иллюстрация групповой видеовстречи и чата"
    >
      <div className="home-scene" aria-hidden="true">
        <div className="home-scene-orbit" />
        <div className="home-scene-caption">
          <span /> На одной волне
        </div>
        <div className="home-call-card">
          <div className="home-call-topbar">
            <span className="home-call-symbol">
              <Video size={16} />
            </span>
            <span>Встреча команды</span>
            <span className="home-call-people">
              <i>А</i>
              <i>М</i>
              <i>Д</i>
              <i>И</i>
            </span>
          </div>
          <div className="home-call-portrait">
            <img
              src="/media/meetspace-group-chat-v5.webp"
              alt=""
              width="1536"
              height="1024"
              fetchPriority="high"
            />
          </div>
          <div className="home-call-controls">
            <span>
              <Mic size={18} />
            </span>
            <span>
              <Video size={18} />
            </span>
            <span className="home-call-hangup">
              <Phone size={18} />
            </span>
            <span>
              <MessageCircle size={18} />
            </span>
          </div>
        </div>
        <div className="home-chat-card">
          <div className="home-chat-heading">
            <MessageCircle size={15} />
            <span>Разговор продолжается</span>
            <ArrowUpRight size={16} />
          </div>
          <div className="home-chat-message">
            <span className="home-chat-avatar">А</span>
            <div>
              <strong>Алексей</strong>
              <p>Есть идея. Обсудим вместе?</p>
            </div>
          </div>
          <div className="home-chat-answer">
            Да, давайте! <span>↗</span>
          </div>
        </div>
      </div>
    </figure>
  );
}
