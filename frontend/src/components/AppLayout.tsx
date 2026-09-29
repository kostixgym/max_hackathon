import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button } from '@maxhub/max-ui';
import { useApi } from '../hooks/useApi';
import { fetchMe, getStartParam } from '../lib/api';
import { Seg, useTheme } from './ui';
import { Icon } from './Icon';

type Mode = 'resident' | 'uk';

export function AppLayout({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const profile = useApi(fetchMe, [location.key]);
  const [canManage, setCanManage] = useState(false);
  const [lastMode, setLastMode] = useState<Mode>('resident');
  const launchHandled = useRef(false);
  const routeMode: Mode | null = location.pathname === '/uk' || location.pathname.startsWith('/uk/')
    ? 'uk'
    : location.pathname === '/' || location.pathname === '/attach' || location.pathname.startsWith('/home')
      ? 'resident' : null;
  const mode = routeMode ?? lastMode;

  useEffect(() => {
    if (routeMode) setLastMode(routeMode);
  }, [routeMode]);

  useEffect(() => {
    window.scrollTo(0, 0);
  }, [location.pathname]);

  useEffect(() => {
    if (!profile.data) return;
    const allowed = profile.data.orgs.length > 0;
    // Keep the last confirmed role if a later request fails, so errors never
    // remove navigation. A successful response can still revoke the role.
    setCanManage(allowed);
    if (!launchHandled.current) {
      launchHandled.current = true;
      if (allowed && getStartParam() === 'uk' && location.pathname === '/') {
        navigate('/uk', { replace: true });
      }
    }
  }, [profile.data, location.pathname, navigate]);

  const openMode = (next: Mode) => {
    launchHandled.current = true;
    setLastMode(next);
    navigate(next === 'uk' ? '/uk' : '/');
  };

  return (
    <div className={`app-shell kit${theme === 'dark' ? ' t-dark' : ''}`}>
      <nav className="app-navigation" aria-label="Главное меню">
        {canManage && <Seg<Mode> value={mode} onChange={openMode} options={[
          { value: 'resident', label: 'Житель' },
          { value: 'uk', label: 'УК' },
        ]} />}
        <Button className="app-menu-button" type="button" variant="ghost" size="medium"
          onClick={() => openMode('resident')}><span className="app-menu-label"><Icon name="menu" small />В меню</span></Button>
      </nav>
      {children}
    </div>
  );
}
