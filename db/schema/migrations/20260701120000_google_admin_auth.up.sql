ALTER TABLE admin_users
ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS picture_url text NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS provider text NOT NULL DEFAULT 'google',
ADD COLUMN IF NOT EXISTS last_login_at timestamptz;

CREATE TABLE IF NOT EXISTS admin_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    created_by_admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    accepted_by_admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS admin_invitations_active_email_idx
ON admin_invitations(email, expires_at)
WHERE accepted_at IS NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS admin_invitations_created_idx
ON admin_invitations(created_at DESC);
