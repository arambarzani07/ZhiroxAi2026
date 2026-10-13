-- Zhirox Remote Control v1 control-plane schema.
-- Not applied automatically. Production deployment requires an explicit migration gate.

create table if not exists public.pc_remote_sessions (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  controller_token_hash text not null,
  state text not null check (state in ('requested','approved','connecting','active','ended','rejected','expired')),
  requested_capabilities jsonb not null default '{}'::jsonb,
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
  constraint pc_remote_session_expiry_bound check (expires_at <= requested_at + interval '30 minutes')
);

create index if not exists pc_remote_sessions_agent_state_idx
  on public.pc_remote_sessions(agent_id, state, requested_at desc);

create table if not exists public.pc_remote_signals (
  id bigint generated always as identity primary key,
  session_id uuid not null references public.pc_remote_sessions(id) on delete cascade,
  sender text not null check (sender in ('agent','controller')),
  kind text not null check (kind in ('offer','answer','ice_candidate','hangup')),
  payload jsonb not null,
  created_at timestamptz not null default now(),
  consumed_at timestamptz
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
  created_at timestamptz not null default now()
);

create index if not exists pc_remote_audit_agent_created_idx
  on public.pc_remote_audit(agent_id, created_at desc);

alter table public.pc_remote_sessions enable row level security;
alter table public.pc_remote_signals enable row level security;
alter table public.pc_remote_audit enable row level security;

-- No direct client policies are created in v1. The Edge Function uses service-role
-- access after explicit custom authentication; browser/client direct table access
-- therefore fails closed.
revoke all on public.pc_remote_sessions from anon, authenticated;
revoke all on public.pc_remote_signals from anon, authenticated;
revoke all on public.pc_remote_audit from anon, authenticated;
