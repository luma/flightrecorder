import { createContext, useContext, useState, type ReactNode } from "react";

const KEY = "flightrecorder_project_scope";

interface ProjectScopeContextValue {
  projectScope: string | null;
  setProjectScope: (id: string | null) => void;
}

const ProjectScopeContext = createContext<ProjectScopeContextValue>({
  projectScope: null,
  setProjectScope: () => {},
});

export function ProjectScopeProvider({ children }: { children: ReactNode }) {
  const [projectScope, setProjectScopeState] = useState<string | null>(() => {
    return sessionStorage.getItem(KEY);
  });
  const setProjectScope = (id: string | null) => {
    setProjectScopeState(id);
    if (id) sessionStorage.setItem(KEY, id);
    else sessionStorage.removeItem(KEY);
  };
  return (
    <ProjectScopeContext.Provider value={{ projectScope, setProjectScope }}>
      {children}
    </ProjectScopeContext.Provider>
  );
}

export function useProjectScope() {
  return useContext(ProjectScopeContext);
}
