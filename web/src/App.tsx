import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom";
import { AppProvider } from "./auth/AuthProvider";
import { PortalBar } from "./components/PortalBar";
import { AuthCallback } from "./pages/AuthCallback";
import { GamePage } from "./pages/GamePage";
import { HomePage } from "./pages/HomePage";

function KeyedGame() {
  const { id } = useParams();
  return <GamePage key={id} id={id!} />;
}

export default function App() {
  return (
    <BrowserRouter>
      <AppProvider>
        <PortalBar />
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/g/:id" element={<KeyedGame />} />
          <Route path="/auth/callback" element={<AuthCallback />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AppProvider>
    </BrowserRouter>
  );
}
