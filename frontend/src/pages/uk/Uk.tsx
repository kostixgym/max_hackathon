import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Btn, Cell, Header, Main, Note, Screen, Seg, Status, Tile, UiList, Card } from '../../components/ui';
import { useApi } from '../../hooks/useApi';
import { becomeDemoStaff, fetchMe, fetchOrgHouses, fetchOrgs, getStartParam } from '../../lib/api';

function RoleSwitch() {
  const navigate = useNavigate();
  return <Seg value="uk" onChange={(v) => v === 'resident' && navigate('/home')} options={[{ value: 'resident', label: 'Жилец' }, { value: 'uk', label: 'УК' }]} />;
}

export function UkHouses() {
  const meState = useApi(() => fetchMe());
  const orgsState = useApi(() => fetchOrgs());
  const org = orgsState.data?.orgs[0];
  const housesState = useApi(() => org ? fetchOrgHouses(org.id) : Promise.resolve({ houses: [] }), [org?.id]);
  const [joining, setJoining] = useState(false);
  const houseSlug = meState.data?.house?.slug ?? getStartParam();

  const joinDemo = async () => {
    if (!houseSlug || joining) return;
    setJoining(true);
    try {
      await becomeDemoStaff(houseSlug);
      window.location.reload();
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Не удалось открыть кабинет демо-УК');
      setJoining(false);
    }
  };

  if (meState.loading || orgsState.loading || housesState.loading) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка кабинета…</MaxTypography.Text></Card></Main></Screen>;
  if (meState.error || orgsState.error || housesState.error) return <Screen><Main><Note kind="neg">Не удалось загрузить кабинет УК.</Note></Main></Screen>;

  const orgs = orgsState.data?.orgs ?? [];
  const houses = housesState.data?.houses ?? [];
  return <Screen>
    <div className="col" style={{ gap: 12, padding: '12px 16px 8px' }}>
      <RoleSwitch />
      <div className="row"><div className="grow"><MaxTypography.Headline className="h2" variant="medium">{org?.name ?? 'Кабинет УК'}</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">Дома, доступные вашей организации</MaxTypography.Text></div>{org && <Status kind="mod">Сотрудник УК</Status>}</div>
    </div>
    <Main style={{ gap: 12 }}>
      <div className="between"><MaxTypography.Headline className="h3" variant="small">Дома</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">{houses.length}</MaxTypography.Text></div>
      {houses.length > 0 ? <UiList>{houses.map((house) => <Cell key={house.id} lead={<Tile icon="building" />}><span style={{ fontWeight: 600 }}>{house.address}</span><MaxTypography.Text className="cap" variant="detail" color="secondary">{house.region}{house.is_demo ? ' · демо-дом' : ''}</MaxTypography.Text></Cell>)}</UiList> : <Note kind="info">У организации пока нет доступных домов.</Note>}
      {orgs.length === 0 && houseSlug && <Btn onClick={joinDemo} disabled={joining}>{joining ? 'Подключаем…' : 'Войти в демо-кабинет УК'}</Btn>}
      {orgs.length === 0 && !houseSlug && <Note kind="info">Откройте мини-приложение по ссылке демо-дома, чтобы получить демо-доступ УК.</Note>}
      <Note kind="info">Список требований и заявки на подтверждение пока не реализованы на backend.</Note>
    </Main>
  </Screen>;
}

export function UkDemands() {
  return <Screen><Header title="Требования собственников" /><Main><Note kind="info">Ручка списка требований пока отвечает 501.</Note></Main></Screen>;
}

export function UkCreateMeeting() {
  return <Screen><Header title="Создание собрания" /><Main><Note kind="info">Backend создаёт собрание только по инициативе на этапе «Требование». Переход от требования пока не реализован.</Note></Main></Screen>;
}

export function UkRequests() {
  return <Screen><Header title="Заявки собственников" /><Main><Note kind="info">Ручек для заявок и их обработки пока нет.</Note></Main></Screen>;
}
