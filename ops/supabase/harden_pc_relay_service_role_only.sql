-- Defense-in-depth hardening for the Zhirox PC relay/control plane.
-- These tables are accessed by the relay Edge Function with service_role.
-- RLS remains enabled, and anon/authenticated receive no direct table privileges.

revoke all privileges on table public.pc_relay_agents from anon, authenticated;
revoke all privileges on table public.pc_relay_commands from anon, authenticated;
revoke all privileges on table public.pc_relay_connector_tokens from anon, authenticated;
revoke all privileges on table public.pc_relay_heartbeats from anon, authenticated;
revoke all privileges on table public.pc_relay_pairing_tokens from anon, authenticated;

revoke all privileges on table public.pc_os_backup_catalog from anon, authenticated;
revoke all privileges on table public.pc_os_events from anon, authenticated;
revoke all privileges on table public.pc_os_integrity_baselines from anon, authenticated;
revoke all privileges on table public.pc_os_snapshots from anon, authenticated;
revoke all privileges on table public.pc_os_update_audit from anon, authenticated;
revoke all privileges on table public.pc_relay_command_approvals from anon, authenticated;

-- enqueue_approved_pc_relay_command remains SECURITY INVOKER and EXECUTE is
-- granted only to service_role by pc_relay_approval_gate.sql.
