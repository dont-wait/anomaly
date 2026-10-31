import React from "react";
import ReactDOM from "react-dom/client";
import { MobileRoot } from "./app/MobileRoot";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <MobileRoot />
  </React.StrictMode>,
);
