import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { Apt, Badge, Btn, Cell, Chip, Foot, Header, Main, Screen, UiInput, UiList } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { fmtNum } from '../../lib/format';
import { flats } from '../../mocks/demo';
import { P } from '../../paths';

// 2. Выбор квартиры
export function Apartment() {
  const [query, setQuery] = useState('4');
  const [entrance, setEntrance] = useState<0 | 1 | 2>(0);
  const [selected, setSelected] = useState(45);

  const found = flats.filter((f) => (!query || String(f.n).includes(query)) && (!entrance || f.entrance === entrance));

  return (
    <Screen>
      <Header title="Выбор квартиры" />
      <Main>
        <label className="field">
          <MaxTypography.Label className="lbl" variant="large-strong">Номер квартиры</MaxTypography.Label>
          <UiInput inputMode="numeric" value={query} onChange={(e) => setQuery(e.target.value.replace(/\D/g, ''))} aria-label="Номер квартиры" iconBefore={<Icon name="search" className="chev" />} />
        </label>
        <div className="row">
          <Chip on={entrance === 0} onClick={() => setEntrance(0)}>
            Все
          </Chip>
          <Chip on={entrance === 1} onClick={() => setEntrance(1)}>
            Подъезд 1
          </Chip>
          <Chip on={entrance === 2} onClick={() => setEntrance(2)}>
            Подъезд 2
          </Chip>
        </div>
        <div className="between" style={{ padding: '4px 4px 0' }}>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Найдено {found.length} квартир</MaxTypography.Text>
          <Badge kind="fact">Площадь из реестра</Badge>
        </div>
        {found.length > 0 && (
          <UiList>
            {found.map((f) => {
              const on = f.n === selected;
              return (
                <Cell key={f.n} lead={<Apt n={f.n} />} trail={<span className={'rd' + (on ? ' on' : '')} />} onClick={() => setSelected(f.n)} style={on ? { background: 'var(--acc-soft)' } : undefined}>
                  <span style={{ fontWeight: 600 }}>Кв. {f.n}</span>
                  <MaxTypography.Text className="cap" variant="detail" color="secondary">Подъезд {f.entrance} · этаж {f.floor} · {fmtNum(f.areaM2)} м²</MaxTypography.Text>
                </Cell>
              );
            })}
          </UiList>
        )}
      </Main>
      <Foot>
        <Btn to={P.confirm}>Выбрать кв. {selected}</Btn>
      </Foot>
    </Screen>
  );
}
