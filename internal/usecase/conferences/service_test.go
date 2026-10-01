package conferences

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

// TestCreateRetriesInviteCollisionAndDoesNotJoinOwner проверяет сценарий «создание Retries Invite Collision и выполняет не Join владелец», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCreateRetriesInviteCollisionAndDoesNotJoinOwner(t *testing.T) {
	userID := uuid.NewString()
	repo := &collisionRepository{}
	service := NewService(repo, &testUserRepository{id: userID}, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
		  - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() (string, error) { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil })
	view, err := service.Create(context.Background(), userID, domain.CreateRequest{Title: " Test meeting "})
	if err != nil {
		t.Fatal(err)
	}
	if repo.attempts != 2 || view.OwnerID != userID || view.Title != "Test meeting" {
		t.Fatal("invite retry or authenticated ownership failed")
	}
	if repo.owner.Role != domain.Owner || repo.owner.JoinedAt != nil || repo.owner.LeftAt != nil || repo.owner.Status != domain.Left {
		t.Fatal("owner membership incorrectly represents an actual join")
	}
}

// collisionRepository реализует постоянное хранение ресурсов компонента через GORM.
// Состав:
//   - repository: встроенный тип, добавляющий свой контракт или данные.
//   - attempts: значение attempts типа int, используемое согласно назначению этой операции.
//   - owner: значение owner типа domain.Participant, используемое согласно назначению этой операции.
type collisionRepository struct {
	repository
	attempts int
	owner    domain.Participant
}

// Create создаёт новое состояние ресурсов компонента по переданным параметрам.
//
// @parameters:
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - conference (domain.Conference): конференция либо её идентификатор, ограничивающий область операции.
//   - owner (domain.Participant): значение owner типа domain.Participant, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *collisionRepository) Create(_ context.Context, conference domain.Conference, owner domain.Participant) (domain.Conference, error) {
	r.attempts++
	r.owner = owner
	if r.attempts == 1 {
		return domain.Conference{}, domain.ErrInviteCollision
	}
	return conference, nil
}

// testUserRepository реализует постоянное хранение ресурсов компонента через GORM.
// Состав:
//   - id: идентификатор обрабатываемого ресурса.
type testUserRepository struct{ id string }

// GetByID читает учётную запись по её идентификатору.
//
// @parameters:
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *testUserRepository) GetByID(_ context.Context, id string) (users.User, error) {
	if id != r.id {
		return users.User{}, apperrors.ErrNotFound
	}
	return users.User{ID: id, Email: "test@example.com"}, nil
}
