-- Add session_idle_ttl_minutes to projects
ALTER TABLE public.projects 
ADD COLUMN session_idle_ttl_minutes integer DEFAULT 120;
