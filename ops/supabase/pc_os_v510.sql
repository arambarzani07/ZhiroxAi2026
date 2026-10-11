-- Zhirox PC OS v5.1.0 additive telemetry/control-plane schema.
-- No existing relay/business rows are modified.
-- All tables are service-role only by default; anon/authenticated receive no grants.

create table if not exists public.pc_os_snapshots (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  captured_at timestamptz not null default now(),
  module text not null,
  severity text not null default 'info' check (severity in ('info','warning','critical')),
  payload jsonb not null default '{}'::jsonb
);

create index if not exists pc_os_snapshots_agent_captured_idx
  on public.pc_os_snapshots(agent_id, captured_at desc);
create index if not exists pc_os_snapshots_module_captured_idx
  on public.pc_os_snapshots(module, captured_at desc);

create table if not exists public.pc_os_events (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  created_at timestamptz not null default now(),
  event_type text not null,
  severity text not null default 'info' check (severity in ('info','warning','critical')),
  payload jsonb not null default '{}'::jsonb
);

create index if not exists pc_os_events_agent_created_idx
  on public.pc_os_events(agent_id, created_at desc);
create index if not exists pc_os_events_type_created_idx
  on public.pc_os_events(event_type, created_at desc);

create table if not exists public.pc_os_integrity_baselines (
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  path text not null,
  sha256 text not null check (sha256 ~ '^[0-9a-fA-F]{64}$'),
  bytes bigint,
  approved_at timestamptz not null default now(),
  approved_by text not null default 'owner',
  metadata jsonb not null default '{}'::jsonb,
  primary key (agent_id, path)
);

create table if not exists public.pc_os_backup_catalog (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  backup_type text not null,
  location text not null,
  created_at timestamptz not null default now(),
  bytes bigint,
  sha256 text,
  status text not null default 'created' check (status in ('created','verified','failed','expired')),
  verified_at timestamptz,
  metadata jsonb not null default '{}'::jsonb,
  check (sha256 is null or sha256 ~ '^[0-9a-fA-F]{64}$')
);

create index if not exists pc_os_backup_catalog_agent_created_idx
  on public.pc_os_backup_catalog(agent_id, created_at desc);

create table if not exists public.pc_os_update_audit (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  created_at timestamptz not null default now(),
  version text not null,
  artifact_sha256 text,
  signature_ok boolean not null default false,
  staged_ok boolean not null default false,
  health_gate_ok boolean not null default false,
  rollback_performed boolean not null default false,
  metadata jsonb not null default '{}'::jsonb,
  check (artifact_sha256 is null or artifact_sha256 ~ '^[0-9a-fA-F]{64}$')
);

create index if not exists pc_os_update_audit_agent_created_idx
  on public.pc_os_update_audit(agent_id, created_at desc);

alter table public.pc_os_snapshots enable row level security;
alter table public.pc_os_events enable row level security;
alter table public.pc_os_integrity_baselines enable row level security;
alter table public.pc_os_backup_catalog enable row level security;
alter table public.pc_os_update_audit enable row level security;

revoke all on table public.pc_os_snapshots from anon, authenticated;
revoke all on table public.pc_os_events from anon, authenticated;
revoke all on table public.pc_os_integrity_baselines from anon, authenticated;
revoke all on table public.pc_os_backup_catalog from anon, authenticated;
revoke all on table public.pc_os_update_audit from anon, authenticated;
