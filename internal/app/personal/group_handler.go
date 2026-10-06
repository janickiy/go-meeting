package personalapp

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type GroupRepository interface {
	CreateGroup(context.Context, string, personal.CreateGroupRequest) (personal.Conversation, bool, error)
	UpdateGroup(context.Context, string, string, personal.UpdateGroupRequest) (personal.Conversation, error)
	GroupMembers(context.Context, string, string) ([]personal.Member, error)
	AddGroupMembers(context.Context, string, string, []string) (personal.Conversation, []personal.Member, error)
	RemoveGroupMember(context.Context, string, string, string) (personal.Conversation, bool, error)
	ChangeGroupRole(context.Context, string, string, string, personal.Role) (personal.Conversation, error)
	TransferGroupOwnership(context.Context, string, string, string) (personal.Conversation, error)
	LeaveGroup(context.Context, string, string) error
	DeleteGroup(context.Context, string, string) ([]string, error)
}
type GroupPresence interface {
	Online(context.Context, []string) (map[string]bool, error)
}

func groupParameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}
func groupResult(c *gin.Context, item personal.Conversation) {
	c.JSON(200, gin.H{"status": "success", "item": item})
}
func (h *Handler) groupEvent(c *gin.Context, id, kind, user string, role personal.Role, extra ...string) {
	if h.Events == nil {
		return
	}
	data := gin.H{"conversationId": id, "type": "group"}
	if user != "" {
		data["userId"] = user
	}
	if role != "" {
		data["role"] = role
	}
	_ = h.Events.PublishConversationTo(c.Request.Context(), id, kind, data, extra...)
}

func (h *Handler) CreateGroup(c *gin.Context) {
	var body personal.CreateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	actor := middleware.UserID(c)
	item, created, err := h.Repo.CreateGroup(c.Request.Context(), actor, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created {
		h.groupEvent(c, item.ID, "conversation.member.added", actor, personal.Owner)
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}
func (h *Handler) UpdateGroup(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	var body personal.UpdateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.UpdateGroup(c.Request.Context(), middleware.UserID(c), id, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.updated", "", "")
	groupResult(c, item)
}
func (h *Handler) GroupMembers(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	items, err := h.Repo.GroupMembers(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.Presence != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, member := range items {
			ids = append(ids, member.ID)
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		online, presenceErr := h.Presence.Online(ctx, ids)
		cancel()
		if err := c.Request.Context().Err(); err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if presenceErr == nil {
			for i := range items {
				if value, known := online[items[i].ID]; known {
					items[i].Online = &value
				}
			}
		}
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}
func (h *Handler) AddGroupMembers(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, added, err := h.Repo.AddGroupMembers(c.Request.Context(), middleware.UserID(c), id, body.UserIDs)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, member := range added {
		h.groupEvent(c, id, "conversation.member.added", member.ID, member.Role)
	}
	groupResult(c, item)
}
func (h *Handler) RemoveGroupMember(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	target, ok := groupParameter(c, "userId")
	if !ok {
		return
	}
	item, changed, err := h.Repo.RemoveGroupMember(c.Request.Context(), middleware.UserID(c), id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if changed {
		h.groupEvent(c, id, "conversation.member.removed", target, "", target)
	}
	groupResult(c, item)
}
func (h *Handler) ChangeGroupRole(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	target, ok := groupParameter(c, "userId")
	if !ok {
		return
	}
	var body struct {
		Role personal.Role `json:"role"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.ChangeGroupRole(c.Request.Context(), middleware.UserID(c), id, target, body.Role)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", target, body.Role)
	groupResult(c, item)
}
func (h *Handler) TransferGroupOwnership(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	target, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	actor := middleware.UserID(c)
	item, err := h.Repo.TransferGroupOwnership(c.Request.Context(), actor, id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", actor, personal.Admin)
	h.groupEvent(c, id, "conversation.member.updated", target, personal.Owner)
	groupResult(c, item)
}
func (h *Handler) LeaveGroup(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	var body struct{}
	if !httpresponse.BindJSON(c, &body, true) {
		return
	}
	actor := middleware.UserID(c)
	if err := h.Repo.LeaveGroup(c.Request.Context(), actor, id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.removed", actor, "", actor)
	c.JSON(200, gin.H{"status": "success"})
}
func (h *Handler) DeleteGroup(c *gin.Context) {
	id, ok := groupParameter(c, "id")
	if !ok {
		return
	}
	former, err := h.Repo.DeleteGroup(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, user := range former {
		h.groupEvent(c, id, "conversation.member.removed", user, "", user)
	}
	c.JSON(200, gin.H{"status": "success"})
}
