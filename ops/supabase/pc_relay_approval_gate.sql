-- Zhirox relay structured-mutation approval gate.
-- Additive only: no existing command/agent rows are modified.
-- The RPC is SECURITY INVOKER: service_role already owns the exact table
-- privileges it needs, so the function never requires privilege elevation.

create table if not exists public.pc_relay_command_approvals (
  id uuid primary key default gen_random_uuid(),
  agent_id text not null references public.pc_relay_agents(agent_id) on delete cascade,
  action text not null,
  args jsonb not null default '{}'::jsonb,
  reason text,
  approved_by text not null default 'owner',
  approved_at timestamptz not null default now(),
  expires_at timestamptz not null,
  used_at timestamptz,
  command_id uuid references public.pc_relay_commands(id) on delete set null,
  check (expires_at > approved_at),
  constraint pc_relay_command_approvals_ttl_check
    check (expires_at <= approved_at + interval '15 minutes')
);

-- Upgrade-safe constraint for projects where the approval table already exists.
do $$
begin
  if not exists (
    select 1
    from pg_constraint
    where conname = 'pc_relay_command_approvals_ttl_check'
      and conrelid = 'public.pc_relay_command_approvals'::regclass
  ) then
    alter table public.pc_relay_command_approvals
      add constraint pc_relay_command_approvals_ttl_check
      check (expires_at <= approved_at + interval '15 minutes');
  end if;
end;
$$;

create index if not exists pc_relay_command_approvals_agent_idx
  on public.pc_relay_command_approvals(agent_id, approved_at desc);
create index if not exists pc_relay_command_approvals_open_idx
  on public.pc_relay_command_approvals(agent_id, expires_at)
  where used_at is null;
create index if not exists pc_relay_command_approvals_command_id_idx
  on public.pc_relay_command_approvals(command_id);

alter table public.pc_relay_command_approvals enable row level security;
revoke all on table public.pc_relay_command_approvals from anon, authenticated;
grant select, insert, update, delete on table public.pc_relay_command_approvals to service_role;

create or replace function public.enqueue_approved_pc_relay_command(
  p_approval_id uuid,
  p_agent_id text,
  p_action text,
  p_args jsonb
) returns jsonb
language plpgsql
security invoker
set search_path = pg_catalog, public
as $$
declare
  a public.pc_relay_command_approvals%rowtype;
  c public.pc_relay_commands%rowtype;
begin
  select * into a
  from public.pc_relay_command_approvals
  where id = p_approval_id
  for update;

  if not found then
    raise exception 'approval_not_found';
  end if;
  if a.used_at is not null then
    raise exception 'approval_already_used';
  end if;
  if a.expires_at <= now() then
    raise exception 'approval_expired';
  end if;
  if a.agent_id <> p_agent_id then
    raise exception 'approval_agent_mismatch';
  end if;
  if a.action <> p_action then
    raise exception 'approval_action_mismatch';
  end if;
  if a.args <> coalesce(p_args, '{}'::jsonb) then
    raise exception 'approval_args_mismatch';
  end if;

  insert into public.pc_relay_commands(agent_id, action, args, status)
  values (p_agent_id, p_action, coalesce(p_args, '{}'::jsonb), 'queued')
  returning * into c;

  update public.pc_relay_command_approvals
  set used_at = now(), command_id = c.id
  where id = a.id;

  return jsonb_build_object(
    'id', c.id,
    'agent_id', c.agent_id,
    'action', c.action,
    'args', c.args,
    'status', c.status,
    'created_at', c.created_at,
    'approval_id', a.id
  );
end;
$$;

revoke all on function public.enqueue_approved_pc_relay_command(uuid,text,text,jsonb) from public, anon, authenticated;
grant execute on function public.enqueue_approved_pc_relay_command(uuid,text,text,jsonb) to service_role;
