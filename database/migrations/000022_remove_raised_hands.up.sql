-- Удаляем агрегаты отключённой функции. Исторические миграции остаются неизменными.
ALTER TABLE participant_analytics
    DROP COLUMN hand_raises,
    DROP COLUMN last_hand_at;
