import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@maxhub/max-ui/dist/styles.css';
import { App } from './App';
import './styles/maxkit.css';
import './styles/app.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
