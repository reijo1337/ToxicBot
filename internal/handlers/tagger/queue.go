package tagger

import (
	"container/heap"
	"context"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

type taggerJob struct {
	tagAt  time.Time
	chatID int64
	token  uint64
}

// taggerQueue — куча-минимум по времени тега. Не потокобезопасна: все операции
// выполняются под Handler.mu.
type taggerQueue []taggerJob

func (q *taggerQueue) Len() int           { return len(*q) }
func (q *taggerQueue) Less(i, j int) bool { return (*q)[i].tagAt.Before((*q)[j].tagAt) }
func (q *taggerQueue) Swap(i, j int)      { (*q)[i], (*q)[j] = (*q)[j], (*q)[i] }

func (q *taggerQueue) Push(x any) {
	job, _ := x.(taggerJob)
	*q = append(*q, job)
}

func (q *taggerQueue) Pop() any {
	old := *q
	out := old[len(old)-1]
	*q = old[:len(old)-1]
	return out
}

// indexOf возвращает позицию задачи чата или -1.
func (q *taggerQueue) indexOf(chatID int64) int {
	for i, job := range *q {
		if job.chatID == chatID {
			return i
		}
	}
	return -1
}

// tagAttempt — снятая с очереди задача и выбранный для неё кандидат.
type tagAttempt struct {
	job    taggerJob
	userID int64
	// epoch — эпоха состава чата на момент выбора кандидата.
	epoch chatEpoch
}

// takeDue снимает ближайшую задачу, если её срок наступил.
// Будущая задача остаётся в очереди нетронутой.
func (h *Handler) takeDue(now time.Time) (tagAttempt, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.queue) == 0 || now.Before(h.queue[0].tagAt) {
		return tagAttempt{}, false
	}

	job, _ := heap.Pop(&h.queue).(taggerJob)

	users := h.users.Users(job.chatID)
	if len(users) == 0 {
		delete(h.jobs, job.chatID)
		return tagAttempt{}, false
	}

	return tagAttempt{job: job}, true
}

// selectTarget связывает одного уникального автора снимка с актуальной задачей и эпохой.
func (h *Handler) selectTarget(job taggerJob, history []chathistory.Entry) (tagAttempt, bool) {
	authors := make(map[int64]struct{}, len(history))
	for _, entry := range history {
		if !entry.FromBot && entry.SenderID > 0 {
			authors[entry.SenderID] = struct{}{}
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.jobCurrentLocked(job) {
		return tagAttempt{}, false
	}
	users := h.users.Users(job.chatID)
	eligible := make([]int64, 0, len(users))
	for _, id := range users {
		if _, ok := authors[id]; ok {
			eligible = append(eligible, id)
		}
	}
	if len(eligible) == 0 {
		return tagAttempt{}, false
	}
	return tagAttempt{
		job:    job,
		userID: eligible[h.random.Intn(len(eligible))],
		epoch:  h.epochs[job.chatID],
	}, true
}

// dropGoneCandidate удаляет кандидата, отсутствие которого подтвердила проверка, если
// за время запроса никто не подтвердил присутствие в чате. Иначе ответ мог устареть:
// например, участник успел вернуться.
func (h *Handler) dropGoneCandidate(ctx context.Context, a tagAttempt) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.epochs[a.job.chatID].outdates(a.epoch, membershipGone) {
		return nil
	}

	return h.removeUserLocked(ctx, a.job.chatID, a.userID)
}

// canSend сообщает, актуальна ли попытка перед отправкой: задача чата не сменилась,
// а кандидат остался в списке. Сеть не держит блокировку, поэтому за время проверки
// или генерации чат могли очистить и наполнить заново с новым сроком.
func (h *Handler) canSend(a tagAttempt) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.jobCurrentLocked(a.job) {
		return false
	}

	_, ok := h.users.Author(a.job.chatID, a.userID)
	return ok
}

// finishJob планирует следующий тег, если задача всё ещё актуальна: за время попытки
// чат могли очистить и наполнить заново, тогда у него уже есть новая задача.
func (h *Handler) finishJob(job taggerJob) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.jobCurrentLocked(job) {
		return
	}

	delete(h.jobs, job.chatID)
	h.scheduleLocked(job.chatID)
}

// jobCurrentLocked сообщает, остаётся ли задача единственной задачей своего чата.
func (h *Handler) jobCurrentLocked(job taggerJob) bool {
	return h.jobs[job.chatID] == job.token
}

func (h *Handler) scheduleLocked(chatID int64) {
	if _, ok := h.jobs[chatID]; ok || len(h.users.Users(chatID)) == 0 {
		return
	}

	h.jobSeq++
	h.jobs[chatID] = h.jobSeq
	heap.Push(&h.queue, taggerJob{tagAt: h.makeTagAt(), chatID: chatID, token: h.jobSeq})
}

func (h *Handler) unscheduleLocked(chatID int64) {
	delete(h.jobs, chatID)

	if i := h.queue.indexOf(chatID); i >= 0 {
		heap.Remove(&h.queue, i)
	}
}
