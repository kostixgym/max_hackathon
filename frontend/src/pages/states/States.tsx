import { Button as MaxButton, Typography as MaxTypography } from '@maxhub/max-ui';
import { Btn, DemoBadge, Foot, Header, Main, Screen, StateHead, Status, Card } from '../../components/ui';
import { Circle } from '../../components/ui';
import { fmtM2 } from '../../lib/format';
import { house, me } from '../../mocks/demo';
import { P } from '../../paths';

function Sk({ w, h, r, style }: { w?: number | string; h: number; r?: number; style?: React.CSSProperties }) {
  return <div className="sk" style={{ width: w, height: h, borderRadius: r, ...style }} />;
}

// Загрузка (скелетон главной)
export function Loading() {
  return (
    <Screen>
      <div aria-busy="true" aria-label="Загрузка" style={{ display: 'contents' }}>
        <div className="col" style={{ gap: 8, padding: '20px 20px 12px' }}>
          <Sk w={140} h={26} />
          <Sk w={230} h={16} />
        </div>
        <Main>
          <Card className="card">
            <div className="row" style={{ gap: 12 }}>
              <Sk w={56} h={56} r={999} />
              <div className="col" style={{ gap: 8, flex: 1 }}>
                <Sk w="60%" h={20} />
                <Sk w="40%" h={24} r={99} />
              </div>
            </div>
            <Sk h={18} />
            <Sk w="80%" h={18} />
            <Sk w="70%" h={18} />
          </Card>
          <Sk w={160} h={22} style={{ margin: '8px 4px 0' }} />
          <Card className="card" style={{ gap: 12 }}>
            <div className="between">
              <Sk w="55%" h={22} />
              <Sk w={70} h={26} r={99} />
            </div>
            <Sk h={8} r={99} />
            <Sk w="50%" h={16} />
          </Card>
          <Card className="card" style={{ gap: 12 }}>
            <div className="between">
              <Sk w="65%" h={22} />
              <Sk w={80} h={26} r={99} />
            </div>
            <Sk w="45%" h={16} />
          </Card>
        </Main>
        <Foot>
          <Sk h={56} r={14} />
        </Foot>
      </div>
    </Screen>
  );
}

// Пусто: в доме нет инициатив
export function EmptyState() {
  return (
    <Screen>
      <div className="col" style={{ gap: 2, padding: '16px 20px 8px' }}>
        <div className="row">
          <MaxTypography.Headline className="h2" style={{ flex: 1 }} variant="medium">
            Мой дом
          </MaxTypography.Headline>
          <DemoBadge />
        </div>
        <MaxTypography.Text className="cap" variant="detail" color="secondary">{house.shortAddress}</MaxTypography.Text>
      </div>
      <Main>
        <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <span className="av">{me.name[0]}</span>
          <div className="grow">
            <MaxTypography.Headline className="h3" variant="small">
              {me.name} · кв. {me.premise.number}
            </MaxTypography.Headline>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Вес голоса {fmtM2(me.weightM2)}</MaxTypography.Text>
          </div>
          <Status kind="ok">Собственник</Status>
        </Card>
        <div className="between" style={{ padding: '6px 4px 0' }}>
          <MaxTypography.Headline className="h3" variant="small">Инициативы дома</MaxTypography.Headline>
        </div>
        <Card className="card" style={{ alignItems: 'center', gap: 12, padding: '32px 24px' }}>
          <Circle icon="bulb" tone="acc" size={72} />
          <MaxTypography.Headline className="h3" style={{ textAlign: 'center' }} variant="small">
            В доме пока нет инициатив
          </MaxTypography.Headline>
          <MaxTypography.Text className="t2" style={{ textAlign: 'center' }} variant="body" color="secondary">
            Предложите первую: камеры, домофон, шлагбаум или свой вопрос. Начнём с опроса соседей.
          </MaxTypography.Text>
        </Card>
      </Main>
      <Foot>
        <Btn icon="plus" to={P.templates}>
          Новая инициатива
        </Btn>
      </Foot>
    </Screen>
  );
}

// Ошибка загрузки с повтором
export function ErrorState({ title = 'Ход опроса', onRetry }: { title?: string; onRetry?: () => void }) {
  return (
    <Screen>
      <Header title={title} />
      <Main style={{ justifyContent: 'center', gap: 14, padding: 24 }}>
        <StateHead icon="offline" tone="neg" title="Не удалось загрузить" text="Похоже, пропал интернет. Ваши ответы не потерялись — попробуйте ещё раз." />
      </Main>
      <Foot>
        <Btn icon="retry" onClick={onRetry ?? (() => window.location.reload())}>
          Повторить
        </Btn>
      </Foot>
    </Screen>
  );
}

// Ошибка авторизации: приложение открыто не из MAX (401 от бэкенда)
export function AuthState({ code = 'AUTH_INIT_DATA' }: { code?: string }) {
  return (
    <Screen>
      <Main style={{ justifyContent: 'center', gap: 14, padding: 24 }}>
        <StateHead
          icon="chat"
          tone="acc"
          title="Откройте приложение из MAX"
          text="Мы не смогли понять, кто вы. Так бывает, если открыть ссылку в обычном браузере. Откройте её в мессенджере MAX — в чате дома или через QR-код."
        />
        <MaxTypography.Text className="cap" style={{ textAlign: 'center' }} variant="detail" color="secondary">
          Код ошибки: {code}
        </MaxTypography.Text>
      </Main>
      <Foot>
        <MaxButton asChild variant="primary" size="large" stretched>
          <a href="https://max.ru">Открыть в MAX</a>
        </MaxButton>
      </Foot>
    </Screen>
  );
}
