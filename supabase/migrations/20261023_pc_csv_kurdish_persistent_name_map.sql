-- Cloud-only Sorani materialized CSV overlay. Original PC SQL and immutable raw CSV remain read-only.
create table if not exists public.pc_csv_kurdish_names (
  agent_id text not null,
  item_name_id text not null,
  original_name text not null,
  kurdish_name text not null,
  status text not null default 'normalized'
    check (status in ('normalized','verified','needs_review')),
  updated_at timestamptz not null default now(),
  primary key (agent_id,item_name_id)
);
alter table public.pc_csv_kurdish_names enable row level security;
revoke all on public.pc_csv_kurdish_names from anon,authenticated;
create table if not exists public.pc_csv_kurdish_manifest (
  agent_id text primary key,
  source_sha256 text not null check (source_sha256 ~ '^[0-9a-f]{64}$'),
  output_sha256 text not null check (output_sha256 ~ '^[0-9a-f]{64}$'),
  storage_path text not null,
  rows integer not null check(rows>0),
  bytes integer not null check(bytes>0),
  names_verified integer not null default 0,
  names_normalized integer not null default 0,
  names_needing_review integer not null default 0,
  updated_at timestamptz not null default now()
);
alter table public.pc_csv_kurdish_manifest enable row level security;
revoke all on public.pc_csv_kurdish_manifest from anon,authenticated;
