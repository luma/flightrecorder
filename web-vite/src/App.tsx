import { Routes, Route } from "react-router-dom";

import Layout from "./components/Layout";
import RequireAuth from "./auth/RequireAuth";
import Dashboard from "./pages/Dashboard";
import Login from "./pages/Login";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Dashboard />} />
      </Route>
    </Routes>
  );
}
