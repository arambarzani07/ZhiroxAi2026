-- Zhirox Remote Control v1 control-plane schema.
-- Not applied automatically. Production deployment requires an explicit migration gate.
-- Screen-only, attended, signaling-only. No input/control capability is enabled here.

create table if not exists public.pc_remote_controller_tokens (
  token_hash text primary key,
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  label text not null default 'remote-controller',
  allow_screen_stream boolean not null default true,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_used_at timestamptz,
  revoked_at timestamptz,
  constraint pc_remote_controller_token_hash_format
    check (token_hash ~ '^[0-9a-f]{64}$'),
  constraint pc_remote_controller_token_label_bound
    check (char_length(label) between 1 and 120),
  constraint pc_remote_controller_token_ttl_bound
    check (expires_at > created_at and expires_at <= created_at + interval '24 hours'),
  constraint pc_remote_controller_token_scope_unique unique (token_hash, agent_id)
);

create index if not exists pc_remote_controller_tokens_active_idx
  on public.pc_remote_controller_tokens(agent_id, expires_at)
  where revoked_at is null and allow_screen_stream;

create table if not exists public.pc_remote_sessions (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  controller_token_hash text not null,
  state text not null check (state in ('requested','approved','connecting','active','ended','rejected','expired')),
  requested_capabilities jsonb not null default '{"screen_stream":true,"mouse":false,"keyboard":false,"clipboard":false,"file_transfer":false,"audio":false}'::jsonb,
  approved_capabilities jsonb not null default '{}'::jsonb,
  requested_at timestamptz not null default now(),
  approved_at timestamptz,
  connected_at timestamptz,
  ended_at timestamptz,
  expires_at timestamptz not null,
  last_activity_at timestamptz not null default now(),
  controller_label text,
  controller_fingerprint text,
  reason text,
  constraint pc_remote_session_controller_scope_fk
    foreign key (controller_token_hash, agent_id)
    references public.pc_remote_controller_tokens(token_hash, agent_id)
    on delete cascade,
  constraint pc_remote_session_expiry_bound
    check (expires_at > requested_at and expires_at <= requested_at + interval '30 minutes'),
  constraint pc_remote_session_requested_screen_only
    check (
      requested_capabilities @> '{"screen_stream":true}'::jsonb
      and requested_capabilities <@ '{"screen_stream":true,"mouse":false,"keyboard":false,"clipboard":false,"file_transfer":false,"audio":false}'::jsonb
    ),
  constraint pc_remote_session_approved_screen_only
    check (
      approved_capabilities = '{}'::jsonb
      or (
        approved_capabilities @> '{"screen_stream":true}'::jsonb
        and approved_capabilities <@ '{"screen_stream":true,"mouse":false,"keyboard":false,"clipboard":false,"file_transfer":false,"audio":false}'::jsonb
      )
    ),
  constraint pc_remote_session_controller_label_bound
    check (controller_label is null or char_length(controller_label) <= 120),
  constraint pc_remote_session_controller_fingerprint_bound
    check (controller_fingerprint is null or char_length(controller_fingerprint) <= 120),
  constraint pc_remote_session_reason_bound
    check (reason is null or char_length(reason) <= 200)
);

create index if not exists pc_remote_sessions_agent_state_idx
  on public.pc_remote_sessions(agent_id, state, requested_at desc);

-- Defense in depth for the one-controller policy. Stale/expired sessions must be
-- transitioned to a terminal state before a replacement request is inserted.
create unique index if not exists pc_remote_sessions_one_live_per_agent_idx
  on public.pc_remote_sessions(agent_id)
  where state in ('requested','approved','connecting','active');

create table if not exists public.pc_remote_signals (
  id bigint generated always as identity primary key,
  session_id uuid not null references public.pc_remote_sessions(id) on delete cascade,
  sender text not null check (sender in ('agent','controller')),
  kind text not null check (kind in ('offer','answer','ice_candidate','hangup')),
  payload jsonb not null,
  created_at timestamptz not null default now(),
  consumed_at timestamptz,
  constraint pc_remote_signal_payload_bound
    check (octet_length(payload::text) <= 262144)
);

create index if not exists pc_remote_signals_session_created_idx
  on public.pc_remote_signals(session_id, id);

create table if not exists public.pc_remote_audit (
  id bigint generated always as identity primary key,
  session_id uuid references public.pc_remote_sessions(id) on delete set null,
  agent_id text not null,
  event_type text not null,
  actor text not null check (actor in ('agent','controller','system')),
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  constraint pc_remote_audit_event_type_bound check (char_length(event_type) between 1 and 80),
  constraint pc_remote_audit_metadata_bound check (octet_length(metadata::text) <= 32768)
);

create index if not exists pc_remote_audit_agent_created_idx
  on public.pc_remote_audit(agent_id, created_at desc);

alter table public.pc_remote_controller_tokens enable row level security;
alter table public.pc_remote_sessions enable row level security;
alter table public.pc_remote_signals enable row level security;
alter table public.pc_remote_audit enable row level security;

-- No direct client policies are created in v1. The Edge Function uses service-role
-- access only after explicit custom authentication. Browser/client direct table
-- access therefore fails closed. RLS-with-no-policy is intentional for these tables.
revoke all on public.pc_remote_controller_tokens from anon, authenticated;
revoke all on public.pc_remote_sessions from anon, authenticated;
revoke all on public.pc_remote_signals from anon, authenticated;
revoke all on public.pc_remote_audit from anon, authenticated;

-- Identity sequences are not client APIs either.
revoke all on sequence public.pc_remote_signals_id_seq from anon, authenticated;
revoke all on sequence public.pc_remote_audit_id_seq from anon, authenticated;
