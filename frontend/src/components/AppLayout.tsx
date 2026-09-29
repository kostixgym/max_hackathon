import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button } from '@maxhub/max-ui';
import { useApi } from '../hooks/useApi';
import { fetchMe, getStartParam } from '../lib/api';
import { Seg, useTheme } from './ui';
import { Icon } from './Icon';

type Mode = 'resident' | 'uk' | 'admin';

export function AppLayout({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const profile = useApi(fetchMe, [location.key]);
  const [availableModes, setAvailableModes] = useState<Mode[]>(['resident']);
  const [lastMode, setLastMode] = useState<Mode>('resident');
  const launchHandled = useRef(false);
  const routeMode: Mode | null = location.pathname === '/admin' ? 'admin' : location.pathname === '/uk' || location.pathname.startsWith('/uk/')
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
    const modes: Mode[] = ['resident'];
    if (profile.data.orgs.length > 0) modes.push('uk');
    if (profile.data.is_admin) modes.push('admin');
    // Keep the last confirmed role if a later request fails, so errors never
    // remove navigation. A successful response can still revoke the role.
    setAvailableModes(modes);
    if (!launchHandled.current) {
      launchHandled.current = true;
      if (modes.includes('uk') && getStartParam() === 'uk' && location.pathname === '/') {
        navigate('/uk', { replace: true });
      }
    }
  }, [profile.data, location.pathname, navigate]);

  const openMode = (next: Mode) => {
    launchHandled.current = true;
    setLastMode(next);
    navigate(next === 'uk' ? '/uk' : next === 'admin' ? '/admin' : '/');
  };

  return (
    <div className={`app-shell kit${theme === 'dark' ? ' t-dark' : ''}`}>
      <nav className="app-navigation" aria-label="Главное меню">
        {availableModes.length > 1 && <Seg<Mode> value={mode} onChange={openMode} options={[
          { value: 'resident', label: 'Житель' },
          ...(availableModes.includes('uk') ? [{ value: 'uk' as const, label: 'УК' }] : []),
          ...(availableModes.includes('admin') ? [{ value: 'admin' as const, label: 'Администратор' }] : []),
        ]} />}
        <Button className="app-menu-button" type="button" variant="ghost" size="medium"
          onClick={() => openMode('resident')}><span className="app-menu-label"><Icon name="menu" small />В меню</span></Button>
      </nav>
      {children}
    </div>
  );
}
