import { Routes, Route } from "react-router-dom";

import Layout from "./components/Layout";
import RequireAuth from "./auth/RequireAuth";
import Dashboard from "./pages/Dashboard";
import Login from "./pages/Login";
import AcceptInvite from "./pages/AcceptInvite";
import LoginError from "./pages/LoginError";
import MCPConsent from "./pages/MCPConsent";
import UsersPage from "./pages/UsersPage";
import AgentsPage from "./pages/AgentsPage";
import DataQuality from "./pages/DataQuality";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/accept-invite" element={<AcceptInvite />} />
      <Route path="/login-error" element={<LoginError />} />
      <Route
        path="/mcp/consent"
        element={
          <RequireAuth>
            <MCPConsent />
          </RequireAuth>
        }
      />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Dashboard />} />
        <Route path="/data-quality" element={<DataQuality />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/agents" element={<AgentsPage />} />
      </Route>
    </Routes>
  );
}
