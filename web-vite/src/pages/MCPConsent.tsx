import { useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../api";
import LogoMark from "../components/LogoMark";

export default function MCPConsent() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const request = searchParams.get("request") ?? "";
  const [allProjects, setAllProjects] = useState(false);
  const [projectKeys, setProjectKeys] = useState<string[]>([]);
  const details = useQuery({
    queryKey: ["mcp-consent", request],
    queryFn: () => api.mcpConsentDetails(request),
    enabled: request.length > 0,
    retry: false,
  });
  const confirm = useMutation({
    mutationFn: () => api.confirmMCPConsent(request, { all_projects: allProjects, project_keys: projectKeys }),
    onSuccess: (resp) => {
      window.location.href = resp.redirect_uri;
    },
  });

  const canConfirm = useMemo(() => allProjects || projectKeys.length > 0, [allProjects, projectKeys]);

  const toggleProject = (projectKey: string) => {
    setProjectKeys((current) =>
      current.includes(projectKey) ? current.filter((value) => value !== projectKey) : [...current, projectKey],
    );
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-dim px-4">
      <div className="w-full max-w-lg space-y-5 rounded-md border border-outline-ghost bg-surface-container-low p-5">
        <div className="flex items-center gap-3">
          <LogoMark size={44} className="text-primary" />
          <div>
            <h1 className="text-xl font-semibold text-on-surface">Allow agent access</h1>
            <p className="text-sm text-on-surface-variant">{details.data?.client_name ?? "MCP Agent"}</p>
          </div>
        </div>

        {!request ? <p className="text-sm text-status-error">Missing authorization request.</p> : null}
        {details.isLoading ? <p className="text-sm text-on-surface-variant">Loading...</p> : null}
        {details.error ? (
          <p className="rounded-md border border-status-error bg-status-error-muted p-3 text-sm text-status-error">
            {details.error instanceof Error ? details.error.message : "Could not load authorization request"}
          </p>
        ) : null}

        {details.data ? (
          <div className="space-y-3">
            <p className="text-sm font-medium text-on-surface">Which project is this agent allowed to access?</p>
            <label className="flex items-center gap-2 rounded-md border border-outline-ghost bg-surface-container px-3 py-2 text-sm text-on-surface">
              <input
                type="checkbox"
                checked={allProjects}
                onChange={(event) => {
                  setAllProjects(event.target.checked);
                  if (event.target.checked) setProjectKeys([]);
                }}
              />
              <span>All Projects</span>
            </label>
            <div className="max-h-72 space-y-2 overflow-auto">
              {details.data.projects.map((project) => (
                <label
                  key={project.project_id}
                  className="flex items-center gap-2 rounded-md border border-outline-ghost bg-surface-container px-3 py-2 text-sm text-on-surface"
                >
                  <input
                    type="checkbox"
                    checked={projectKeys.includes(project.project_id)}
                    disabled={allProjects}
                    onChange={() => toggleProject(project.project_id)}
                  />
                  <span className="min-w-0 flex-1 truncate">{project.display_name}</span>
                  <code className="text-xs text-on-surface-variant">{project.project_id}</code>
                </label>
              ))}
            </div>
            {confirm.error ? (
              <p className="text-sm text-status-error">{confirm.error instanceof Error ? confirm.error.message : "Authorization failed"}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => navigate("/")} className="btn-secondary">
                Cancel
              </button>
              <button
                type="button"
                onClick={() => confirm.mutate()}
                disabled={!canConfirm || confirm.isPending}
                className="btn-primary disabled:opacity-50"
              >
                {confirm.isPending ? "Confirming..." : "Confirm"}
              </button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
