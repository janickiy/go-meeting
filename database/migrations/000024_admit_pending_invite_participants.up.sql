-- Existing pending invite holders should not remain stranded after direct link admission.
-- Explicitly rejected/kicked memberships and closed/scheduled meetings are unchanged.
UPDATE conference_participants p
SET status = 'joined', admission_state = 'admitted',
    admission_decided_at = now(), admission_version = admission_version + 1,
    joined_at = now(), left_at = NULL, media_policy_version = media_policy_version + 1
FROM conferences c
WHERE p.conference_id = c.id AND c.status IN ('created', 'active')
  AND p.status = 'waiting' AND p.admission_state = 'waiting';
