import { useEffect, useState } from 'react';
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom';
import { MaxUI } from '@maxhub/max-ui';
import { ThemeContext } from './components/ui';
import { Catalog } from './pages/Catalog';
import { P } from './paths';
import { applicationRoutes, groups } from './routes';

type Theme = 'light' | 'dark';

function systemTheme(): Theme {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function savedTheme(): Theme | null {
  try {
    const t = localStorage.getItem('theme');
    return t === 'light' || t === 'dark' ? t : null;
  } catch {
    return null;
  }
}

export function App() {
  const [theme, setTheme] = useState<Theme>(() => savedTheme() ?? systemTheme());

  useEffect(() => {
    document.body.classList.toggle('dark', theme === 'dark');
  }, [theme]);

  const changeTheme = (t: Theme) => {
    setTheme(t);
    try {
      localStorage.setItem('theme', t);
    } catch {
      // приватный режим — тема просто не запомнится
    }
  };

  return (
      <MaxUI colorScheme={theme}>
      <ThemeContext.Provider value={{ theme, changeTheme }}>
      {/* HashRouter: мини-приложение открывается по одному URL из MAX, сервер не знает о путях */}
      <HashRouter>
        <Routes>
          <Route path="/__screens" element={<Catalog />} />
          {applicationRoutes.map(({ path, Component }) => <Route key={path} path={path} element={<Component />} />)}
          {/* The concrete template URL is handled by /templates/:code, which supplies the code param. */}
          {groups.flatMap((g) => g.screens).filter(({ path }) => path !== P.templateForm).map(({ path, Component }) => (
            <Route key={path} path={path} element={<Component />} />
          ))}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </HashRouter>
      </ThemeContext.Provider>
      </MaxUI>
  );
}
