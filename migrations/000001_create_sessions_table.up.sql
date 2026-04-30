create table sessions (
    session_id text primary key,
    messages   jsonb not null default '[]'::jsonb,
    updated_at timestamptz not null default now()
);