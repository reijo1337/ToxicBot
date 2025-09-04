package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/marcboeker/go-duckdb/v2"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/internal/message"
	"github.com/reijo1337/ToxicBot/pkg/mapper"
)

var (
	operationTypeFromDomain = map[stats.OperationType]string{
		stats.OnTextOperationType:     "on_text",
		stats.OnStickerOperationType:  "on_sticker",
		stats.OnVoiceOperationType:    "on_voice",
		stats.OnUserJoinOperationType: "on_user_join",
		stats.OnUserLeftOperationType: "on_user_left",
		stats.PersonalOperationType:   "personal",
		stats.TaggerOperationType:     "tagger",
	}
	operationTypeToDomain = mapper.InvertMap(operationTypeFromDomain)

	generationTypeFromDomain = map[message.GenerationStrategy]string{
		message.ByListGenerationStrategy: "by_list",
		message.AiGenerationStrategy:     "ai",
	}
	generationTypeToDomain = mapper.InvertMap(generationTypeFromDomain)
)

type responseLogRow struct {
	Date       time.Time               `db:"date"`
	Type       string                  `db:"type"`
	ChatIDHash []byte                  `db:"chat_id_hash"`
	UserIDHash []byte                  `db:"user_id_hash"`
	Extra      JSON[*responseLogExtra] `db:"extra"`
}

type responseLogExtra struct {
	TextGenerationType string `json:"text_generation_type"`
}

type ResponseLogStorage struct {
	connGetter connGetter
}

func NewResponseLogStorage(connGetter connGetter) *ResponseLogStorage {
	return &ResponseLogStorage{
		connGetter: connGetter,
	}
}

func responseLogRowFromDomain(event stats.Response) (*responseLogRow, error) {
	opType, found := operationTypeFromDomain[event.OperationType]
	if !found {
		return nil, fmt.Errorf("unknown operation type: %v", event.OperationType)
	}

	extra, err := responseLogExtraFromDomain(event.Extra)
	if err != nil {
		return nil, fmt.Errorf("failed to convert extra: %w", err)
	}

	return &responseLogRow{
		Date:       event.Date,
		Type:       opType,
		ChatIDHash: event.ChatIDHash,
		UserIDHash: event.UserIDHash,
		Extra:      JSON[*responseLogExtra]{t: extra},
	}, nil
}

func responseLogExtraFromDomain(extra *stats.ResponseExtra) (*responseLogExtra, error) {
	if extra == nil {
		return nil, nil
	}

	generationType, found := generationTypeFromDomain[extra.TextGenerationType]
	if !found {
		return nil, fmt.Errorf("unknown text generation type: %v", extra.TextGenerationType)
	}

	return &responseLogExtra{
		TextGenerationType: generationType,
	}, nil
}

// CREATE

func (r *ResponseLogStorage) Create(ctx context.Context, event stats.Response) error {
	row, err := responseLogRowFromDomain(event)
	if err != nil {
		return fmt.Errorf("failed to convert event to row: %w", err)
	}

	const query = `
insert into response_log (
	"date"
	,"type"
	,chat_id_hash
	,user_id_hash
	,extra
) values (
	:date
	,:type
	,:chat_id_hash
	,:user_id_hash
	,:extra
)`

	_, err = r.connGetter.Get(ctx).NamedExecContext(ctx, query, row)
	if err != nil {
		return fmt.Errorf("failed to create response log: %w", err)
	}

	return nil
}

// READ

type totalStat struct {
	ByOpTypeStat  duckdb.Composite[map[string]uint64] `db:"op_type_stats"`
	ByGenTypeStat duckdb.Composite[map[string]uint64] `db:"gen_type_stats"`
	BulledChats   uint64                              `db:"bullied_chats"`
	BulledUsers   uint64                              `db:"bullied_users"`
	OldestDate    time.Time                           `db:"oldest_date"`
}

func (r *ResponseLogStorage) GetTotalStat(ctx context.Context) (*stats.TotalStat, error) {
	const query = `
with op_type_stats as (
	select
		rl."type" k
		,count(*) v
	from
		response_log rl
	group by
		rl."type"
), op_type_stats_json as (
	select
		json_group_object(k,v) op_type_stats
	from 
		op_type_stats
), gen_type_stats as (
	select
		extra->>'text_generation_type' k
		,count(*) v
	from
		response_log rl
	where
		rl.extra is not null
	group by
		extra->>'text_generation_type'
), gen_type_stats_json as (
	select
		json_group_object(k,v) gen_type_stats
	from 
		gen_type_stats
), common_stats as (
	select
		count(distinct chat_id_hash) bullied_chats
		,count(distinct user_id_hash) bullied_users
		,min("date") oldest_date
	from
		response_log
) select
	op_type_stats
	,gen_type_stats
	,bullied_chats
	,bullied_users
	,oldest_date
from
	op_type_stats_json, gen_type_stats_json, common_stats`
	var totalStat totalStat
	err := r.connGetter.Get(ctx).GetContext(ctx, &totalStat, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get total stat: %w", err)
	}

	out := stats.TotalStat{
		ByOpTypeStat: make(map[stats.OperationType]uint64, len(totalStat.ByOpTypeStat.Get())),
		ByGenTypeStat: make(
			map[message.GenerationStrategy]uint64,
			len(totalStat.ByGenTypeStat.Get()),
		),
		BulledChats: totalStat.BulledChats,
		BulledUsers: totalStat.BulledUsers,
		OldestDate:  totalStat.OldestDate,
	}

	for k, v := range totalStat.ByOpTypeStat.Get() {
		dto, found := operationTypeToDomain[k]
		if !found {
			return nil, fmt.Errorf("unknown operation type: %v", k)
		}
		out.ByOpTypeStat[dto] = v
	}

	for k, v := range totalStat.ByGenTypeStat.Get() {
		dto, found := generationTypeToDomain[k]
		if !found {
			return nil, fmt.Errorf("unknown generation type: %v", k)
		}
		out.ByGenTypeStat[dto] = v
	}

	return &out, nil
}

type detailedStat struct {
	ChatNumber    uint64                              `db:"chat_number"`
	BulledUsers   uint64                              `db:"bullied_users"`
	ByOpTypeStat  duckdb.Composite[map[string]uint64] `db:"op_type_stats"`
	ByGenTypeStat duckdb.Composite[map[string]uint64] `db:"gen_type_stats"`
}

func (r *ResponseLogStorage) GetDetailedStat(
	ctx context.Context,
	date time.Time,
) ([]stats.DetailedStat, error) {
	const query = `
with op_type_stats as (
	select	
		chat_id_hash
		,"type" k
		, count(*) v
	from
		response_log
	where
		"date" = $1::date
	group by
		chat_id_hash, "type"
), op_type_stats_json as (
	select
		chat_id_hash
		,json_group_object(k,v) op_type_stats
	from 
		op_type_stats
	group by chat_id_hash 
), gen_type_stats as (
	select
		chat_id_hash
		,extra->>'text_generation_type' k
		,count(*) v
	from
		response_log
	where
		"date" = $1::date
		and
		extra is not null
	group by
		chat_id_hash, extra->>'text_generation_type'
), gen_type_stats_json as (
	select
		chat_id_hash
		,json_group_object(k,v) gen_type_stats
	from 
		gen_type_stats
	group by chat_id_hash
), common_stats as (
	select
		chat_id_hash
		,count(distinct user_id_hash) bullied_users
	from
		response_log
	where
		"date" = $1::date
	group by
		chat_id_hash
) select
	row_number() OVER () chat_number
	,cs.bullied_users 
	,otsj.op_type_stats 
	,gtsj.gen_type_stats
from
	common_stats cs
	join gen_type_stats_json gtsj on cs.chat_id_hash = gtsj.chat_id_hash 
	join op_type_stats_json otsj on cs.chat_id_hash = otsj.chat_id_hash`

	dbStats := []detailedStat{}

	err := r.connGetter.Get(ctx).SelectContext(ctx, &dbStats, query, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get detailed stat: %w", err)
	}

	if len(dbStats) == 0 {
		return nil, nil
	}

	out := make([]stats.DetailedStat, 0, len(dbStats))

	for _, dbStat := range dbStats {
		stat := stats.DetailedStat{
			ChatNumber:   dbStat.ChatNumber,
			BulledUsers:  dbStat.BulledUsers,
			ByOpTypeStat: make(map[stats.OperationType]uint64, len(dbStat.ByOpTypeStat.Get())),
			ByGenTypeStat: make(
				map[message.GenerationStrategy]uint64,
				len(dbStat.ByGenTypeStat.Get()),
			),
		}

		for k, v := range dbStat.ByOpTypeStat.Get() {
			dto, found := operationTypeToDomain[k]
			if !found {
				return nil, fmt.Errorf("unknown operation type: %v", k)
			}
			stat.ByOpTypeStat[dto] = v
		}

		for k, v := range dbStat.ByGenTypeStat.Get() {
			dto, found := generationTypeToDomain[k]
			if !found {
				return nil, fmt.Errorf("unknown generation type: %v", k)
			}
			stat.ByGenTypeStat[dto] = v
		}

		out = append(out, stat)
	}

	return out, nil
}
