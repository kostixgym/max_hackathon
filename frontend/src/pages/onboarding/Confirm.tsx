import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { Apt, Btn, Cell, DemoBadge, Foot, Header, Kv, LinkBtn, Main, Note, Screen, StateHead, Status, Steps, Tile, UiList, Card } from '../../components/ui';
import { fmtNum } from '../../lib/format';
import { house, me } from '../../mocks/demo';
import { P } from '../../paths';

const apt = me.premise;

// 3. Подтверждение собственника
export function Confirm() {
  return (
    <Screen>
      <Header title="Подтверждение" />
      <Main>
        <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12, padding: '12px 16px' }}>
          <Apt n={apt.number} />
          <div className="grow">
            <span style={{ fontWeight: 600 }}>
              Кв. {apt.number} · {fmtNum(apt.areaM2)} м²
            </span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              Подъезд {apt.entrance} · этаж {apt.floor}
            </MaxTypography.Text>
          </div>
          <Status kind="none">Гость</Status>
        </Card>
        <MaxTypography.Text className="t" style={{ padding: '0 4px' }} variant="body">
          Подтвердите, что вы собственник, — тогда ваш голос будет учитываться. Попробуйте способы по очереди:
        </MaxTypography.Text>
        <UiList>
          <Cell lead={<Tile icon="phone" />} chevron onClick={() => {}}>
            <span style={{ fontWeight: 600 }}>1. Поделиться номером</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Сверим с номером в реестре. Быстрее всего</MaxTypography.Text>
          </Cell>
          <Cell lead={<Tile icon="receipt" />} chevron onClick={() => {}}>
            <span style={{ fontWeight: 600 }}>2. Лицевой счёт + сумма из квитанции</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Оба числа есть в последней квитанции</MaxTypography.Text>
          </Cell>
          <Cell lead={<Tile icon="people" />} chevron to={P.ownerList}>
            <span style={{ fontWeight: 600 }}>3. Выбрать себя в списке собственников</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Заявку проверит УК</MaxTypography.Text>
          </Cell>
        </UiList>
        <LinkBtn style={{ padding: '0 4px' }} to={P.homeGuest}>
          Я не собственник
        </LinkBtn>
      </Main>
      {house.isDemo && (
        <Foot>
          <div className="row" style={{ gap: 8, paddingBottom: 4 }}>
            <DemoBadge />
            <MaxTypography.Text className="cap" variant="detail" color="secondary">проверка не нужна</MaxTypography.Text>
          </div>
          <Btn to={P.home}>Подтвердить (демо)</Btn>
        </Foot>
      )}
    </Screen>
  );
}

const owners = [
  { id: 'o1', name: 'Смирнова А. В.', letter: 'А', share: '1/2', weight: 26.15 },
  { id: 'o2', name: 'Смирнов Д. В.', letter: 'Д', share: '1/2', weight: 26.15, gradient: 'linear-gradient(155deg,#bf97ff 6%,#526eff 84%)' },
];

// 3б. Выбрать себя в списке собственников
export function OwnerList() {
  const [selected, setSelected] = useState('o1');
  return (
    <Screen>
      <Header title={`Собственники кв. ${apt.number}`} />
      <Main>
        <div className="between" style={{ padding: '0 4px' }}>
          <MaxTypography.Text className="t2" variant="body" color="secondary">Выберите себя. Имена скрыты частично.</MaxTypography.Text>
          <span className="b b-fact">Реестр</span>
        </div>
        <UiList>
          {owners.map((o) => {
            const on = o.id === selected;
            return (
              <Cell key={o.id} style={on ? { background: 'var(--acc-soft)' } : undefined} onClick={() => setSelected(o.id)} lead={<span className="av" style={{ width: 44, height: 44, fontSize: 17, ...(o.gradient ? { background: o.gradient } : {}) }}>
                  {o.letter}
                </span>} trail={<span className={'rd' + (on ? ' on' : '')} />}>
                  <span style={{ fontWeight: 600 }}>{o.name}</span>
                  <MaxTypography.Text className="cap" variant="detail" color="secondary">
                    Доля {o.share} · голос {fmtNum(o.weight)} м²
                  </MaxTypography.Text>
              </Cell>
            );
          })}
        </UiList>
        <Note kind="info">Заявку проверит УК — обычно за 1–3 рабочих дня. Пока идёт проверка, вы — Гость.</Note>
        <LinkBtn style={{ padding: '0 4px' }}>Меня нет в списке</LinkBtn>
        <LinkBtn style={{ padding: '0 4px' }} to={P.homeGuest}>
          Я не собственник
        </LinkBtn>
      </Main>
      <Foot>
        <Btn to={P.pending}>Отправить заявку в УК</Btn>
      </Foot>
    </Screen>
  );
}

// 3в. Ожидает УК
export function Pending() {
  return (
    <Screen>
      <Header title="Подтверждение" />
      <Main style={{ gap: 16, paddingTop: 24 }}>
        <StateHead icon="clock" tone="warn" title="Заявка ждёт проверки УК" text="Обычно проверяют за 1–3 рабочих дня. Мы пришлём сообщение в MAX, когда УК ответит." />
        <Card className="card" style={{ gap: 10 }}>
          <Kv k="Квартира">кв. {apt.number}</Kv>
          <Kv k="Способ">Выбор в списке</Kv>
          <Kv k="Собственник">Смирнова А. В., доля 1/2</Kv>
          <Kv k="Отправлено">25 сентября, 14:20</Kv>
          <Kv k="Статус">
            <Status kind="paper">Ждёт проверки УК</Status>
          </Kv>
        </Card>
        <Note kind="info">
          Пока вы — <b>Гость</b>: видите инициативы дома, но не голосуете.
        </Note>
      </Main>
      <Foot>
        <Btn to={P.homeGuest}>Перейти к дому</Btn>
        <Btn kind="text">Отменить заявку</Btn>
      </Foot>
    </Screen>
  );
}

// 3г. Отклонено с причиной
export function Rejected() {
  return (
    <Screen>
      <Header title="Подтверждение" />
      <Main style={{ gap: 16, paddingTop: 24 }}>
        <StateHead icon="close" tone="neg" title="УК отклонила заявку" text="Так бывает, если данные в реестре устарели. Попробуйте другой способ или напишите в УК." />
        <Card className="card" style={{ gap: 8 }}>
          <MaxTypography.Label className="lbl" variant="large-strong">Причина от УК</MaxTypography.Label>
          <MaxTypography.Text className="t" variant="body">«Лицевой счёт не совпадает с квартирой 45. Проверьте номер в квитанции за август».</MaxTypography.Text>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{house.uk} · 26 сентября</MaxTypography.Text>
        </Card>
        <UiList>
          <Cell lead={<Tile icon="phone" />} chevron onClick={() => {}}>
            <span style={{ fontWeight: 600 }}>Поделиться номером</span>
          </Cell>
          <Cell lead={<Tile icon="person" />} chevron to={P.ownerList}>
            <span style={{ fontWeight: 600 }}>Выбрать себя в списке</span>
          </Cell>
        </UiList>
      </Main>
      <Foot>
        <Btn to={P.confirm}>Попробовать другой способ</Btn>
        <Btn kind="text">Написать в УК</Btn>
      </Foot>
    </Screen>
  );
}

// 3д. Уже подтверждён за другим аккаунтом
export function Taken() {
  return (
    <Screen>
      <Header title="Подтверждение" />
      <Main style={{ gap: 16, paddingTop: 24 }}>
        <StateHead icon="personX" tone="warn" title="Этот собственник уже подтверждён за другим аккаунтом" text="Одна доля — один аккаунт, чтобы голос не посчитали дважды." />
        <Card className="card" style={{ gap: 12 }}>
          <Steps items={['Если это вы с другого телефона — откройте приложение там.', 'Если нет — сообщите в УК. Она проверит и отвяжет чужой аккаунт.']} />
        </Card>
      </Main>
      <Foot>
        <Btn>Сообщить в УК</Btn>
        <Btn kind="text" to={P.ownerList}>
          Выбрать другого собственника
        </Btn>
      </Foot>
    </Screen>
  );
}
