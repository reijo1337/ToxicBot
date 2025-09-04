create type operation_type as enum (
    'on_text'
    ,'on_sticker'
    ,'on_voice'
    ,'on_user_join'
    ,'on_user_left'
    ,'personal'
    ,'tagger'
);

create table response_log (
    "date" date not null
    ,"type" operation_type not null
    ,chat_id_hash blob not null
    ,user_id_hash blob not null
    ,extra json
);

create index if not exists response_log_date_idx on response_log ("date");