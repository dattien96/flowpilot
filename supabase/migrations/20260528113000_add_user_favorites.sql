create table if not exists user_favorites (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references auth.users(id) on delete cascade,
  target_type text not null check (target_type in ('workflow', 'step')),
  target_id text not null,
  default_project_id uuid references projects(id) on delete set null,
  created_at timestamptz not null default now(),
  
  unique(user_id, target_type, target_id)
);

-- Enable RLS
alter table user_favorites enable row level security;

-- Admin Policy (Like other tables in this app, allow admins to read/write their own favorites, or since it's an admin app, everything)
create policy "Admins can manage their own favorites"
  on user_favorites
  for all
  to authenticated
  using (auth.uid() = user_id)
  with check (auth.uid() = user_id);
