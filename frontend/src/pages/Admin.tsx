import { useState } from 'react';
import { useApi } from '../hooks/useApi';
import { fetchAdminOrgs, searchAdminUsers, setOrgStaff, type AdminUser } from '../lib/api';
import { Btn, Card, Header, Main, Note, Screen } from '../components/ui';

export function AdminPanel() {
  const orgsState = useApi(() => fetchAdminOrgs());
  const [query, setQuery] = useState('');
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [maxUserId, setMaxUserId] = useState<string | null>(null);
  const [orgId, setOrgId] = useState('');
  const [role, setRole] = useState<'operator' | 'admin'>('operator');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  const findUsers = async () => {
    setError(''); setMessage(''); setMaxUserId(null);
    try { setUsers((await searchAdminUsers(query)).users); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Не удалось найти пользователя'); }
  };
  const changeRole = async (action: 'grant' | 'revoke') => {
    if (maxUserId === null || !orgId || busy) return;
    setBusy(true); setError(''); setMessage('');
    try { await setOrgStaff(maxUserId, orgId, role, action); setMessage(action === 'grant' ? 'Доступ в организацию выдан.' : 'Доступ к организации отозван.'); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Не удалось изменить роль'); }
    finally { setBusy(false); }
  };

  const orgs = orgsState.data?.orgs ?? [];
  return <Screen><Header title="Администрирование" nav={null} /><Main style={{ gap: 14 }}>
    <Card className="card"><b>Сотрудники УК</b><span>Найдите зарегистрированный аккаунт по MAX ID, затем выберите организацию и роль.</span></Card>
    {orgsState.error && <Note kind="neg">Недостаточно прав или не удалось загрузить организации.</Note>}
    {error && <Note kind="neg">{error}</Note>}{message && <Note kind="info">{message}</Note>}
    <div style={{ display: 'grid', gap: 10 }}>
      <label>MAX ID пользователя<input inputMode="numeric" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Например, 123456789" minLength={2} style={fieldStyle} /></label>
      <Btn onClick={() => void findUsers()} disabled={busy || query.trim().length < 2}>Найти пользователя</Btn>
    </div>
    {users.length > 0 && <Card className="card" style={{ gap: 8 }}><b>Найденные аккаунты</b>{users.map((user) => <button key={user.max_user_id} type="button" onClick={() => setMaxUserId(user.max_user_id)} style={{ ...choiceStyle, fontWeight: maxUserId === user.max_user_id ? 700 : 400 }}>{user.max_user_id}{maxUserId === user.max_user_id ? ' · выбран' : ''}</button>)}</Card>}
    {users.length === 0 && query.length >= 2 && !error && <Note kind="info">Пользователи не найдены. Аккаунт появится здесь после первого входа в приложение.</Note>}
    {maxUserId !== null && <Card className="card" style={{ gap: 12 }}>
      <b>Доступ для MAX ID {maxUserId}</b>
      <label>Организация<select value={orgId} onChange={(e) => setOrgId(e.target.value)} style={fieldStyle}><option value="">Выберите организацию</option>{orgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}</select></label>
      <label>Роль<select value={role} onChange={(e) => setRole(e.target.value as 'operator' | 'admin')} style={fieldStyle}><option value="operator">Оператор</option><option value="admin">Администратор УК</option></select></label>
      <Btn disabled={busy || !orgId} onClick={() => void changeRole('grant')}>{busy ? 'Сохраняем…' : 'Выдать роль'}</Btn>
      <Btn kind="secondary" disabled={busy || !orgId} onClick={() => void changeRole('revoke')}>Отозвать доступ к организации</Btn>
    </Card>}
  </Main></Screen>;
}

const fieldStyle: React.CSSProperties = { width: '100%', boxSizing: 'border-box', minHeight: 44, marginTop: 6, padding: '10px 12px', borderRadius: 12, border: '1px solid var(--divider-primary)', background: 'var(--background-secondary)', color: 'var(--text-primary)', font: 'inherit' };
const choiceStyle: React.CSSProperties = { textAlign: 'left', padding: 10, border: 0, borderRadius: 8, background: 'var(--background-primary)', color: 'var(--text-primary)', font: 'inherit' };
