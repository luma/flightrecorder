DROP INDEX IF EXISTS admin_invitations_created_idx;
DROP INDEX IF EXISTS admin_invitations_active_email_idx;
DROP TABLE IF EXISTS admin_invitations;

ALTER TABLE admin_users
DROP COLUMN IF EXISTS last_login_at,
DROP COLUMN IF EXISTS provider,
DROP COLUMN IF EXISTS picture_url,
DROP COLUMN IF EXISTS name;
