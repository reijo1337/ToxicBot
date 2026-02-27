create table if not exists chat_settings (
    chat_id              integer primary key
    ,threshold_count     integer
    ,threshold_time_ns   integer
    ,cooldown_ns         integer
    ,sticker_react_chance real
    ,voice_react_chance   real
    ,ai_chance            real
    ,updated_at          datetime not null default current_timestamp
);
