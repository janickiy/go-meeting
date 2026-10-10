package postgres

import "gorm.io/gorm"

// Notifications share the message transaction: retries, failures and attachments
// cannot create a notice without a committed message. Groups are bounded by the
// existing member limit; mute, inactive members and the sender are excluded.
func createConversationNotifications(tx *gorm.DB, messageID string) error {
	return tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
		SELECT gen_random_uuid(),member.user_id,'chat.message',
		 jsonb_build_object('conversationId',c.id,'conversationType',c.type,'messageId',m.id,
		  'isReply',COALESCE(reply.sender_user_id=member.user_id,FALSE)),
		 'conversation-message:'||m.id::text,m.created_at
		FROM conversation_messages m JOIN conversations c ON c.id=m.conversation_id AND c.deleted_at IS NULL
		JOIN conversation_members member ON member.conversation_id=c.id
		JOIN users u ON u.id=member.user_id AND u.guest_conference_id IS NULL
		LEFT JOIN conversation_messages reply ON reply.id=m.reply_to_id AND reply.conversation_id=c.id AND reply.deleted_at IS NULL
		WHERE m.id=? AND m.deleted_at IS NULL AND member.user_id<>m.sender_user_id
		 AND member.left_at IS NULL AND member.hidden_at IS NULL AND member.notifications_enabled
		 AND m.sequence>member.history_cleared_through
		ON CONFLICT(user_id,dedup_key) DO NOTHING`, messageID).Error
}

// Resource IDs are compared as text so malformed historical JSON fails closed
// rather than breaking the whole notification feed. Payloads carry no message
// text or attachments; opening the target always checks the current permissions.
func notificationVisibilitySQL(alias string) string {
	return `NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp
		WHERE cp.user_id=` + alias + `.user_id AND cp.conference_id::text=` + alias + `.payload->>'conferenceId'
		AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
		AND (` + alias + `.type<>'chat.message' OR (
		 EXISTS(SELECT 1 FROM conversations c
		 JOIN conversation_members member ON member.conversation_id=c.id AND member.user_id=` + alias + `.user_id
		 JOIN conversation_messages m ON m.conversation_id=c.id AND m.id::text=` + alias + `.payload->>'messageId'
		 WHERE c.id::text=` + alias + `.payload->>'conversationId' AND c.deleted_at IS NULL
		 AND member.left_at IS NULL AND member.hidden_at IS NULL AND member.notifications_enabled
		 AND m.deleted_at IS NULL AND m.sender_user_id<>member.user_id AND m.sequence>member.history_cleared_through)
		 OR (COALESCE(` + alias + `.payload->>'conversationId','')='' AND
		 EXISTS(SELECT 1 FROM chat_messages m JOIN conference_participants p ON p.conference_id=m.conference_id
		 WHERE m.id::text=` + alias + `.payload->>'messageId' AND m.conference_id::text=` + alias + `.payload->>'conferenceId'
		 AND p.user_id=` + alias + `.user_id AND p.admission_state='admitted' AND p.status IN ('joined','left')
		 AND m.deleted_at IS NULL AND m.sender_user_id<>p.user_id))))`
}
