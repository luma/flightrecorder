ALTER TABLE projects
ADD COLUMN IF NOT EXISTS funnels jsonb NOT NULL DEFAULT '[]'::jsonb;
