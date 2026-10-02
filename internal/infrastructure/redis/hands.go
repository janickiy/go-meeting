package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

const handLifetime = time.Hour

// Hands хранит поднятые руки в Redis с ограничением количества и времени жизни.
// @params
//   - client: клиент внешнего сервиса или транспорта компонента.
//   - prefix: ограниченный префикс объектов, относящихся к одной операции.
type Hands struct {
	client *goredis.Client
	prefix string
}

// NewHands создаёт и связывает зависимости компонента Hands, используемого в поднятых руках и временных реакциях участников.
//
// @args
//   - client (*goredis.Client): клиент внешнего сервиса или транспорта компонента.
//   - namespace (string): изолированное пространство Redis-ключей и каналов приложения или теста.
//
// @return:
//   - результат 1 (*Hands): созданный компонент с переданными зависимостями.
func NewHands(client *goredis.Client, namespace string) *Hands {
	return &Hands{client: client, prefix: namespace + ":hands:"}
}

var raiseHandScript = goredis.NewScript(`
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',ARGV[1]-ARGV[2])
local previous=redis.call('ZSCORE',KEYS[1],ARGV[3])
if previous then return {previous,0} end
if redis.call('ZCARD',KEYS[1]) >= 500 then return {-1,0} end
redis.call('ZADD',KEYS[1],ARGV[1],ARGV[3])
redis.call('PEXPIRE',KEYS[1],ARGV[2])
return {ARGV[1],1}`)

// Raise сохраняет поднятую руку в Redis с ограничением количества и времени хранения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//
// @return:
//   - результат 1 (domain.Hand): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Hands) Raise(ctx context.Context, conferenceID, participantID string) (domain.Hand, bool, error) {
	values, err := raiseHandScript.Run(ctx, s.client, []string{s.prefix + conferenceID}, time.Now().UnixMilli(), handLifetime.Milliseconds(), participantID).Slice()
	if err != nil {
		return domain.Hand{}, false, err
	}
	var milliseconds int64
	switch value := values[0].(type) {
	case string:
		milliseconds, _ = strconv.ParseInt(value, 10, 64)
	case int64:
		milliseconds = value
	}
	if milliseconds < 0 {
		return domain.Hand{}, false, apperrors.New(apperrors.ErrConflict, "too many raised hands")
	}
	return domain.Hand{ParticipantID: participantID, RaisedAt: time.UnixMilli(milliseconds).UTC()}, values[1].(int64) == 1, nil
}

// Lower удаляет активную поднятую руку из Redis.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Hands) Lower(ctx context.Context, conferenceID, participantID string) (bool, error) {
	n, err := s.client.ZRem(ctx, s.prefix+conferenceID, participantID).Result()
	return n > 0, err
}

// List возвращает ограниченный список поднятых рук и реакций комнаты с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 ([]domain.Hand): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Hands) List(ctx context.Context, conferenceID string) ([]domain.Hand, error) {
	key := s.prefix + conferenceID
	pipe := s.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(time.Now().Add(-handLifetime).UnixMilli(), 10))
	result := pipe.ZRangeWithScores(ctx, key, 0, 499)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	hands := make([]domain.Hand, 0, len(result.Val()))
	for _, entry := range result.Val() {
		hands = append(hands, domain.Hand{ParticipantID: entry.Member.(string), RaisedAt: time.UnixMilli(int64(entry.Score)).UTC()})
	}
	return hands, nil
}
