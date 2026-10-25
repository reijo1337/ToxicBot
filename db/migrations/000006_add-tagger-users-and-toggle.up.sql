create table tagger_users (
    chat_id integer not null,
    user_id integer not null,
    author  text    not null,
    primary key (chat_id, user_id)
) without rowid;

alter table chat_settings add column tagger_enabled integer;
