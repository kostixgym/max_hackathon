# API description

## `GET /api/v1/healthz`

Проверяет, что HTTP-процесс запущен.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "status": "ok"
}
```

## `GET /api/v1/readyz`

Проверяет, что сервис готов принимать запросы и соединение с PostgreSQL доступно.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "status": "ready"
}
```

Если PostgreSQL недоступен, возвращает `503 Service Unavailable`:

```json
{
  "error": {
    "code": "not_ready",
    "message": "Сервис временно не готов"
  }
}
```

## `GET /api/v1/me`

Проверяет `initData`, создаёт или находит пользователя MAX и возвращает его данные. Если приложение
открыто по ссылке дома, также возвращает сводку этого дома.

**Request headers:**

- `X-Max-Init-Data` — подписанный `initData` от MAX;
- в локальном `DEV_MODE`: `X-Dev-User-Id` и необязательный `X-Dev-Start-Param`.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "user": {
    "id": "0199...",
    "first_name": "Анна"
  },
  "dev_mode": false,
  "house": {
    "id": "0199...",
    "slug": "demo-house",
    "address": "г. Казань, ул. Демонстрационная, д. 1 (демо-дом)",
    "region": "Республика Татарстан",
    "is_demo": true,
    "premises_count": 61,
    "registry_version": 1,
    "total_area_m2": "3000.00",
    "thresholds": {
      "demand_m2": "300.00",
      "quorum_above_m2": "1500.00",
      "two_thirds_m2": "2000.00"
    }
  },
  "memberships": [
    {
      "id": "0199...",
      "role": "owner",
      "status": "verified",
      "method": "demo",
      "house": {
        "id": "0199...",
        "slug": "demo-house",
        "address": "г. Казань, ул. Демонстрационная, д. 1 (демо-дом)",
        "region": "Республика Татарстан",
        "is_demo": true
      },
      "premise": {
        "id": "0199...",
        "number": "45",
        "kind": "residential",
        "entrance": 3,
        "floor": 2,
        "display_area_m2": "52.30"
      },
      "owner": {
        "id": "0199...",
        "masked_name": "Иванов И. И.",
        "kind": "person",
        "share": {"numerator": 1, "denominator": 2},
        "weight_m2": "26.15"
      }
    }
  ],
  "orgs": [
    {"id": "0199...", "name": "ООО «Демо-УК»", "type": "uk", "role": "operator"}
  ]
}
```

`orgs` — организации, где пользователь работает сотрудником: по ним мини-приложение показывает переключатель
«Жилец / УК». Всегда массив, пустой у жителя.

Если приложение открыто без ссылки дома, поле `house` равно `null`. При неверном или устаревшем
`initData` возвращается `401 Unauthorized`. `memberships` всегда является массивом. Для привязки без
выбранного собственника поле `owner` равно `null`, а до подтверждения `method` может быть `null`.

## `GET /api/v1/houses/{slug}`

Возвращает публичную сводку дома и рассчитанные пороги 10%, кворума и 2/3. Требует авторизации через MAX.

**Path parameter:** `slug` — публичный идентификатор ссылки дома.

**Request headers:** `X-Max-Init-Data`; в локальном `DEV_MODE` — `X-Dev-User-Id`.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "id": "0199...",
  "slug": "demo-house",
  "address": "г. Казань, ул. Демонстрационная, д. 1 (демо-дом)",
  "region": "Республика Татарстан",
  "is_demo": true,
  "premises_count": 61,
  "registry_version": 1,
  "total_area_m2": "3000.00",
  "thresholds": {
    "demand_m2": "300.00",
    "quorum_above_m2": "1500.00",
    "two_thirds_m2": "2000.00"
  }
}
```

Если дом не найден, возвращается `404 Not Found` с кодом ошибки `house_not_found`.

## `GET /api/v1/premises/{premiseID}/owners`

Возвращает собственников помещения из текущей версии реестра. Нужен для выбора `owner_id` перед
`request-owner-verification`. Полное ФИО и телефон не возвращаются.

Доступ разрешён:
- жителю с **подтверждённой** привязкой к этому помещению (проверена квитанция);
- собственнику этого помещения (привязка ожидает подтверждения или подтверждена);
- сотруднику управляющей организации дома.

Гостю (привязка без проверки) список не отдаётся: гостем можно заявить любую квартиру, и иначе по очереди
собирались бы фамилии с инициалами всех собственников дома.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "owners": [
    {
      "id": "0199...",
      "masked_name": "Иванов И. И.",
      "kind": "person",
      "share": {"numerator": 1, "denominator": 2},
      "weight_m2": "26.15"
    }
  ]
}
```

Возвращает `403 forbidden` без подходящей привязки и `404 premise_not_found`, если помещения нет.

## `GET /api/v1/houses/{houseID}/meeting-officer-candidates`

Возвращает замаскированный список собственников дома для выбора председателя и секретаря собрания.
Список нужен только тому, кто организует собрание, поэтому доступ разрешён:
- инициатору активной инициативы этого дома на **пути B** (подтверждённому собственнику);
- сотруднику управляющей организации дома (путь A).

Остальные собственники, в том числе инициатор инициативы на пути A, получают `403 forbidden`.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "owners": [
    {
      "id": "0199...",
      "masked_name": "Иванов И. И.",
      "kind": "person",
      "share": {"numerator": 1, "denominator": 2},
      "weight_m2": "26.15",
      "premise": {"id": "0199...", "number": "45"}
    }
  ]
}
```

Возвращает `403 forbidden` без требуемой роли и `404 house_not_found`, если дома нет.

---

## Минимальный API сквозного MVP

Маршруты ниже составляют минимальный контракт для сценария
«привязка к дому → инициатива → опрос → выбор пути A/B → собрание → итог».
Реализованные помечены **«Реализовано»**, у остальных контракт ещё может уточняться. Ошибки всегда приходят в общем формате
`{"error": {"code": "…", "message": "…"}}`. Ошибка в одном поле формы дополнительно называет его:
`{"error": {"code": "invalid_params", "message": "Заполните поле «Количество камер»", "field": "camera_count"}}`.
`message` можно показывать пользователю как есть. Тело запроса — не больше 64 КБ, иначе `413 request_too_large`.

В документах идея жителя называется `Initiative`, поэтому API использует `/initiatives`, а не
`/issues`. Отдельная заявка в УК с фотографией (`IssueReport`) находится вне текущего MVP.

### Иерархия опроса, выбора пути и официального голосования

```text
Инициатива
└─ предварительный опрос внутри MAX-приложения (PollVote, юридической силы нет)
   ├─ путь A: поддержка ≥ 10%
   │  └─ требование в УК → УК создаёт собрание
   │     └─ если УК просрочила 45 дней → инициатор может перейти на путь B
   └─ путь B: самостоятельная организация без УК
      └─ инициатор создаёт собрание

Собрание
├─ онлайн в ГИС ЖКХ
│  ├─ собственник отмечает у нас «я проголосовал» — только для трекера
│  └─ официальный итог переносится из ГИС агрегированными значениями
└─ бумажный бюллетень — альтернативный канал
   └─ решения переносятся в систему после завершения голосования
```

`PUT /initiatives/{id}/my-vote` — голос в предварительном опросе нашего приложения. Он не является
официальным голосом ОСС. В основном онлайн-сценарии официальный голос собственник отдаёт в ГИС ЖКХ,
а наше приложение только помогает организовать процесс и показывает трекер. Отметка «проголосовал
онлайн» не содержит варианта голоса и никогда не участвует в подсчёте результата.

### `GET /api/v1/houses/{houseID}/premises?number={number}`

Находит помещение по номеру, чтобы пользователь мог привязаться к квартире.

**Request body:** отсутствует.

**Response `200 OK`:** список `{id, number, kind, entrance, floor, display_area_m2}` без персональных данных собственников.

### `POST /api/v1/memberships`

Создаёт привязку текущего пользователя к выбранному помещению с ролью гостя.

**Request body:**

```json
{
  "house_id": "0199...",
  "premise_id": "0199..."
}
```

**Response `201 Created`:** `{id, house_id, premise_id, role: "guest", status: "pending"}`.

### `POST /api/v1/memberships/{id}/verify/demo`

Подтверждает собственника в синтетическом демо-доме. В обычном доме маршрут недоступен.

**Request body:** отсутствует.

**Response `200 OK`:** подтверждённая привязка `{id, role: "owner", status: "verified", method: "demo"}`.

### `POST /api/v1/houses/{slug}/demo-membership`

**Реализовано.** Ярлык демо-дома: подтверждает текущего пользователя собственником квартиры за один шаг, без
`POST /memberships`. В обычном доме недоступен. `{slug}` — пригласительная ссылка дома, как в `start_param`.

**Request body:**

```json
{
  "premise_number": "43",
  "owner_index": 2
}
```

`owner_index` — номер собственника квартиры по порядку, с 1, по умолчанию 1. Нужен в квартирах с совместной
собственностью, чтобы тестировщики получили разные записи.

**Response `200 OK`:** `{membership_id, premise, role: "owner", status: "verified", method: "demo"}`.

**Ошибки:**
- `403 not_demo` — дом не демо;
- `404 house_not_found`, `404 premise_not_found`;
- `409 owner_taken` — этот собственник уже подтверждён за другим аккаунтом (инвариант 9).

### `GET /api/v1/templates`

**Реализовано.** Возвращает последние версии шаблонов инициатив. В MVP шаблон один — «Видеонаблюдение». Шаблоны — данные
платформы без персональных данных, их видит любой вошедший пользователь.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "templates": [
    {
      "code": "video_surveillance",
      "name": "Видеонаблюдение",
      "version": 1,
      "description": "Камеры в подъездах: где ставим, кто хранит записи, как оплачиваем."
    }
  ]
}
```

### `GET /api/v1/templates/{code}`

**Реализовано.** Возвращает форму шаблона, вопросы повестки и правила большинства.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "code": "video_surveillance",
  "name": "Видеонаблюдение",
  "version": 1,
  "description": "Камеры в подъездах: где ставим, кто хранит записи, как оплачиваем.",
  "params_schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "additionalProperties": false,
    "required": ["camera_count", "payment_method"],
    "properties": {
      "placement": {
        "type": "string",
        "title": "Где ставим камеры",
        "description": "Например: входы в подъезды, лифтовые холлы, детская площадка",
        "maxLength": 300
      },
      "camera_count": {"type": "integer", "title": "Количество камер", "minimum": 1, "maximum": 1000},
      "estimated_cost_rub": {
        "type": "integer",
        "title": "Ориентир по стоимости, ₽",
        "description": "Оборудование и монтаж по предварительной оценке",
        "minimum": 0,
        "maximum": 100000000
      },
      "payment_method": {
        "type": "string",
        "title": "Способ оплаты",
        "enum": ["management_bill", "special_assessment"]
      },
      "records_access": {
        "type": "string",
        "title": "Кто имеет доступ к записям",
        "enum": ["management_company", "contractor", "house_council"]
      }
    }
  },
  "ui_schema": {
    "order": ["placement", "camera_count", "estimated_cost_rub", "payment_method", "records_access"],
    "widgets": {"placement": "textarea", "payment_method": "select", "records_access": "select"},
    "enum_titles": {
      "payment_method": {
        "management_bill": "Строкой в квитанции УК",
        "special_assessment": "Разовым целевым сбором"
      },
      "records_access": {
        "management_company": "Управляющая компания",
        "contractor": "Подрядчик, который обслуживает камеры",
        "house_council": "Председатель совета дома"
      }
    }
  },
  "agenda_items": [
    {
      "position": 1,
      "text": "Избрать председателя и секретаря общего собрания и наделить их полномочиями по подсчёту голосов",
      "majority_rule": "majority_of_participants",
      "legal_reference": "ЖК РФ, ст. 46 ч. 1"
    },
    {
      "position": 2,
      "text": "Установить видеонаблюдение в подъездах дома (монтаж, хранение записей и оплата — по проекту, приложенному к материалам собрания)",
      "majority_rule": "two_thirds_of_all",
      "legal_reference": "ЖК РФ, ч. 1 ст. 46, п. 3 ч. 2 ст. 44"
    }
  ]
}
```

**Форма.** `params_schema` — JSON Schema Draft 2020-12 в объёме, который обязан поддержать фронтенд: плоские поля типов
`string`, `integer`, `number`, `boolean` и ключевые слова `title`, `description`, `enum`, `required`, `minimum`, `maximum`,
`minLength`, `maxLength`. Сервер отказывается загружать шаблон с другими ключевыми словами, поэтому неизвестного фронтенду
поля не будет.

**Отображение.** `ui_schema` влияет только на отображение и не участвует в проверке:
- `order` — порядок полей;
- `widgets` — вид поля, если он отличается от обычного: `textarea` — многострочный текст, `select` — выпадающий список;
- `enum_titles` — подписи значений `enum`. Отправлять нужно само значение (`management_bill`), а не подпись.

**Значения.** Необязательное поле без значения в `params` не передаётся: `null` — ошибка. Значения `params` сервер
проверяет по `params_schema` той версии шаблона, по которой создаётся инициатива.

**Ошибки:** `404 template_not_found`.

### `GET /api/v1/houses/{houseID}/initiatives`

**Реализовано.** Возвращает инициативы дома, новые сверху, не больше 100. Список видят подтверждённые жители дома и
сотрудники его УК. Черновик видят только его инициатор и автор (решение 76 в 04), остальным инициатива видна с запуска опроса.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "initiatives": [
    {
      "id": "0199...",
      "title": "Камеры в подъездах",
      "stage": "poll",
      "path": null,
      "poll_ends_at": "2026-10-03T18:00:00+03:00",
      "created_at": "2026-09-26T18:00:00+03:00",
      "is_initiator": false
    }
  ]
}
```

**Ошибки:** `403 not_member` — пользователь не подтверждён в этом доме (неверный `houseID` — тоже `403`).

### `POST /api/v1/houses/{houseID}/initiatives`

Создаёт инициативу из шаблона и фиксирует текущую версию реестра. Житель создаёт черновик,
подтверждённый собственник может стать инициатором.

**Реализовано для собственника.** Черновик жителя — позже. В MVP есть один шаблон: `video_surveillance`. Повестка
начинается с процедурного вопроса об избрании председателя и секретаря, потом идёт вопрос о видеонаблюдении.
`params` проверяются по форме шаблона (`GET /templates/{code}`), для «Видеонаблюдения» обязательны `camera_count` и
`payment_method`. Один пользователь создаёт не больше 10 инициатив за сутки.

**Request body:**

```json
{
  "template_code": "video_surveillance",
  "title": "Камеры в подъездах",
  "description": "Установить камеры во всех подъездах",
  "params": {
    "camera_count": 6,
    "payment_method": "management_bill"
  }
}
```

**Response `201 Created`:**

```json
{
  "id": "0199...",
  "house_id": "0199...",
  "title": "Камеры в подъездах",
  "description": "Установить камеры во всех подъездах",
  "stage": "draft",
  "poll_ends_at": null,
  "is_initiator": true,
  "registry_version": 1,
  "agenda_items": [
    {"position": 1, "text": "Избрать председателя и секретаря…", "majority_rule": "majority_of_participants",
     "legal_reference": "ЖК РФ, ст. 46 ч. 1"},
    {"position": 2, "text": "Установить видеонаблюдение…", "majority_rule": "two_thirds_of_all",
     "legal_reference": "ЖК РФ, ч. 1 ст. 46, п. 3 ч. 2 ст. 44"}
  ]
}
```

`is_initiator` означает, что текущий пользователь — инициатор.

**Ошибки:**
- `400 invalid_request` — пустое название или неверные поля запроса;
- `400 invalid_params` — значения формы не подходят к шаблону. `error.field` называет поле, `message` объясняет, что не так,
  например `Поле «Количество камер»: значение не больше 1000`. Без `field` — `params` не объект;
- `403 not_owner`;
- `404 template_not_found`;
- `409 no_registry` — у дома нет применённой версии реестра;
- `429 too_many_initiatives`.

### `GET /api/v1/initiatives/{id}`

**Реализовано.** Возвращает карточку инициативы: поля, форму шаблона со значениями, повестку, пороги, голос текущего
пользователя и разрешённые ему действия. Карточку видят подтверждённые жители дома и сотрудники его УК, черновик — только
его инициатор и автор.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "id": "0199...",
  "house_id": "0199...",
  "title": "Камеры в подъездах",
  "description": "Установить камеры во всех подъездах",
  "stage": "poll",
  "path": null,
  "poll_ends_at": "2026-10-03T18:00:00+03:00",
  "created_at": "2026-09-26T18:00:00+03:00",
  "is_initiator": false,
  "template": {"code": "video_surveillance", "name": "Видеонаблюдение", "version": 1},
  "params": {"camera_count": 6, "payment_method": "management_bill"},
  "registry_version": 1,
  "total_area_m2": "3000.00",
  "thresholds": {"demand_m2": "300.00", "quorum_above_m2": "1500.00", "two_thirds_m2": "2000.00"},
  "agenda_items": [
    {"position": 1, "text": "Избрать председателя и секретаря…", "majority_rule": "majority_of_participants",
     "legal_reference": "ЖК РФ, ст. 46 ч. 1"},
    {"position": 2, "text": "Установить видеонаблюдение…", "majority_rule": "two_thirds_of_all",
     "legal_reference": "ЖК РФ, ч. 1 ст. 46, п. 3 ч. 2 ст. 44"}
  ],
  "my_vote": {
    "choice": "for",
    "weight_m2": "17.25",
    "official_channel": "paper",
    "willing_to_help": true,
    "updated_at": "2026-09-26T18:05:00+03:00"
  },
  "allowed_actions": [
    {"code": "edit", "allowed": false, "reason_code": "not_implemented"},
    {"code": "start_poll", "allowed": false, "reason_code": "not_initiator"},
    {"code": "cast_poll_vote", "allowed": true},
    {"code": "select_path_a", "allowed": false, "reason_code": "not_implemented"},
    {"code": "select_path_b", "allowed": false, "reason_code": "not_implemented"},
    {"code": "create_meeting", "allowed": false, "reason_code": "not_implemented"},
    {"code": "cancel", "allowed": false, "reason_code": "not_implemented"}
  ]
}
```

- `params` показываются по форме шаблона: подписи полей — из `GET /templates/{template.code}`.
- Пороги и `total_area_m2` считаются от снимка реестра инициативы (`registry_version`), а не от текущего реестра.
- `my_vote` — голос текущего пользователя в опросе, `null`, если он не голосовал. Если у пользователя несколько квартир,
  `weight_m2` — сумма по ним.

**Действия.** `allowed_actions` всегда содержит все известные клиенту действия в одном порядке. Фронтенд определяет
состояние кнопки только по `code` и `allowed`, отображаемый текст локализуется на фронтенде. `reason_code` — стабильный
машинный код причины запрета, у разрешённого действия его нет. Сервер всё равно повторно проверяет право при выполнении
действия.

Коды действий: `edit`, `start_poll`, `cast_poll_vote`, `select_path_a`, `select_path_b`, `create_meeting`, `cancel`.

Сейчас работают два правила:
- `start_poll` — инициатор, черновик. Иначе `not_initiator`, `poll_already_started` или `wrong_stage` (инициатива отменена);
- `cast_poll_vote` — идёт опрос, срок не вышел, пользователь — подтверждённый собственник. Иначе `wrong_stage`,
  `poll_finished` (срок вышел, решение 77) или `owner_verification_required`.

Остальные действия пока приходят с причиной `not_implemented`: их ручки появятся в шагах 1.4–1.6, и кнопка включится
без изменений на фронтенде. Такую кнопку фронтенд прячет или показывает неактивной с подписью «скоро».

Коды причин: `owner_verification_required`, `not_initiator`, `wrong_stage`, `poll_already_started`, `poll_finished`,
`not_implemented`.
Зарезервированы для следующих шагов: `poll_not_finished`, `support_not_reached`, `path_not_selected`,
`active_meeting_exists`.

**Ошибки:**
- `404 initiative_not_found` — нет такой инициативы или это чужой черновик;
- `403 not_member` — пользователь не подтверждён в доме инициативы.

**Будет в Д4 (Дима):**
- поля карточки `demand` — `{id, status, uk_due_at, overdue}` или `null`, и `meeting` — `{id, status}` или `null`;
- правило `select_path_a`: инициатор, стадия опроса, поддержка от 10%, иначе `support_not_reached`;
- правило `create_meeting`: сотрудник УК, который ведёт эту инициативу (решение 79), стадия `demand`, нет активного
  собрания, иначе `staff_only` или `active_meeting_exists`.

### `POST /api/v1/initiatives/{id}/start-poll`

Запускает предварительный опрос и создаёт задачи рассылки подтверждённым собственникам.

**Реализовано.**
- Запускает только инициатор.
- Приглашения с кнопками голосования приходят в чат с ботом.
- В тихие часы (22:00–9:00 по часовому поясу дома) рассылка ждёт утра.
- В демо-доме приглашение уходит только инициатору.
- По окончании срока голоса больше не принимаются, а инициатор получает итог в чат (решение 77).

**Request body:**

```json
{
  "ends_at": "2026-10-02T18:00:00+03:00"
}
```

`ends_at` — в формате RFC 3339, от часа до 30 дней от текущего момента. Без тела или без поля опрос длится 7 дней.

**Response `200 OK`:** `{id, stage: "poll", poll_ends_at}` и остальные поля инициативы, как при создании, но без повестки.

**Ошибки:**
- `400 invalid_poll_duration` или `400 invalid_request`;
- `403 not_initiator`;
- `404 initiative_not_found`;
- `409 wrong_stage` — опрос уже запущен.

### `PUT /api/v1/initiatives/{id}/my-vote`

Создаёт или изменяет голос текущего собственника в предварительном опросе.

**Request body:**

```json
{
  "choice": "for",
  "official_channel": "gosuslugi",
  "willing_to_help": false
}
```

`choice` принимает `for` или `against`. Анкетные поля используются только для голоса `for`.

**Реализовано.** Анкета:
- `official_channel`: `gosuslugi` или `paper`;
- если анкетных полей в запросе нет, сохранённые ответы остаются: повторный голос «за» кнопкой в чате их не стирает;
- голос `against` анкету сбрасывает.

Пользователь с несколькими квартирами в доме голосует всеми сразу, веса складываются. В чате тот же голос отдаётся
кнопками приглашения.

**Response `200 OK`:** `{choice, weight_m2, premises, updated_at}`. Пример: `"weight_m2": "26.15"`, `"premises": "43"`.

**Ошибки:**
- `400 invalid_request`;
- `403 not_owner`;
- `404 initiative_not_found`;
- `409 poll_closed` — опрос завершён: вышел срок или инициатива ушла дальше;
- `409 not_in_snapshot` — записи собственника нет в версии реестра инициативы.

### `GET /api/v1/initiatives/{id}/poll`

Возвращает прогресс предварительного опроса в м² без раскрытия чужих вариантов голосования.

**Реализовано.** Прогресс видят подтверждённые жители дома и сотрудники его УК, остальным — `403 not_member`. Чужой
черновик — `404 initiative_not_found`, как в карточке.

**Request body:** отсутствует.

**Response `200 OK`:** `{for_m2, against_m2, total_m2, demand_m2, demand_reached, poll_ends_at}` и дополнительно
`{initiative_id, title, stage, for_percent, votes_for, votes_against, thresholds: {demand_m2, quorum_above_m2, two_thirds_m2}}`.
Площади — строки с точкой и двумя знаками (`"63.50"`), пороги считаются от общей площади снимка реестра инициативы.

## Путь A: требование, кабинет УК, собрание

Объём до 30.09 — путь A в демо-доме ([plan-do-30-09.md](plan-do-30-09.md), раздел 0). У каждой ручки отмечен статус:
- **Реализовано**;
- **Каркас (шаг)** — маршрут есть и до этого шага плана отвечает `501 not_implemented`;
- **Вне объёма до 30.09** — уходит в «Известные ограничения» README (последний раздел).

Стадии инициативы на пути A:
1. `poll` → `demand`: создано требование, выбран путь A;
2. → `meeting`: УК создала собрание;
3. → `completed`: итог зафиксирован.

Переходы делает `initiatives.SetStageTx` в транзакции того, кто создаёт требование, собрание или итог.

**Кто действует как УК (решение 79).** Сотрудник управляющей организации дома. В демо-доме все тестировщики —
сотрудники одной демо-УК, поэтому там сотрудник видит в кабинете и ведёт только свои инициативы, а уведомления по
ним получает только сам инициатор.

```text
опрос, «за» не меньше 10%
└─ POST /initiatives/{id}/demand               → стадия demand, путь A
   └─ POST /demands/{id}/mark-delivered        → срок УК — 45 дней
      └─ сотрудник УК: POST /initiatives/{id}/meetings → стадия meeting, бюллетени по снимку реестра
         ├─ POST /meetings/{id}/ballots/receive     — бюллетень получен
         ├─ PUT /ballots/{id}/decisions             — решения, после окончания голосования
         ├─ GET /meetings/{id}/result-preview
         └─ POST /meetings/{id}/finalize            → стадия completed, итог записан один раз
            └─ GET /meetings/{id}/protocol.pdf
```

### `POST /api/v1/initiatives/{id}/demand`

**Каркас (Д1).** Выбирает путь A и создаёт требование к УК провести собрание (ст. 45 ч. 6 ЖК). Вызывает инициатор
на стадии опроса при поддержке не меньше 10% площади.

**Request body:** `{"channel": "paper"}`, где `channel` — `paper` или `gosuslugi_dom`.

**Response `201 Created`:**

```json
{
  "id": "0199...",
  "initiative_id": "0199...",
  "channel": "paper",
  "status": "draft",
  "support_m2": "1240.00",
  "created_at": "2026-09-27T12:00:00+03:00"
}
```

**Ошибки:**
- `403 not_initiator`;
- `404 initiative_not_found`;
- `409 wrong_stage`, `409 support_not_reached`, `409 demand_exists`.

### `GET /api/v1/demands/{id}`

**Каркас (Д1).** Требование для экрана инициатора и для кабинета УК.

**Response `200 OK`:** `{id, initiative_id, channel, status, support_m2, delivered_at, uk_due_at, overdue}`.
`status` — `draft` или `delivered`. `overdue` считается при чтении: срок `uk_due_at` прошёл, а собрания нет.

**Ошибки:** `403 forbidden` — не инициатор и не сотрудник УК, который ведёт инициативу; `404 demand_not_found`.

### `POST /api/v1/demands/{id}/mark-delivered`

**Каркас (Д1).** Инициатор отмечает, что требование передано в УК. С этого момента идёт срок 45 дней.

**Request body:** `{"delivered_at": "2026-10-03T12:00:00+03:00"}` — поле необязательно, по умолчанию сейчас.

**Response `200 OK`:** как `GET /demands/{id}`, с `status: "delivered"` и `uk_due_at` = `delivered_at` + 45 дней.

**Ошибки:** `403 not_initiator`, `409 already_delivered`.

### `GET /api/v1/demands/{id}/pdf`

**Каркас (Д3).** PDF требования: адресат (УК), дом, повестка, поддержка в м² против порога 10%, таблица подписей без
ФИО (решение 20). Доступ — у инициатора и у сотрудника УК, который ведёт инициативу. Ответ — `application/pdf`.

## Кабинет УК

### `POST /api/v1/houses/{slug}/demo-staff`

**Реализовано.** Делает пользователя сотрудником (`operator`) демо-УК. Работает только в демо-доме, повторный вызов
ничего не меняет. `{slug}` — пригласительная ссылка дома.

**Request body:** отсутствует.

**Response `200 OK`:** `{"org": {"id": "0199...", "name": "ООО «Демо-УК»", "type": "uk"}, "role": "operator"}`.

**Ошибки:** `403 not_demo`, `404 house_not_found`, `409 no_org`.

### `GET /api/v1/orgs`

**Реализовано.** Организации, где пользователь работает сотрудником. Те же данные приходят в `GET /me` полем `orgs`.

**Response `200 OK`:** `{"orgs": [{"id": "0199...", "name": "ООО «Демо-УК»", "type": "uk", "role": "operator"}]}`.

### `GET /api/v1/orgs/{orgID}/houses`

**Реализовано.** Дома организации.

**Response `200 OK`:** `{"houses": [{"id": "0199...", "address": "…", "region": "…", "is_demo": true}]}`.

**Ошибки:** `403 not_staff` — чужая организация; `404 org_not_found` — неверный id.

### `GET /api/v1/orgs/{orgID}/demands`

**Каркас (К2 после Д1).** Входящие требования. В демо-доме — только по своим инициативам (решение 79).

**Response `200 OK`:**

```json
{
  "demands": [
    {
      "id": "0199...",
      "initiative_id": "0199...",
      "initiative_title": "Камеры в подъездах",
      "house": {"id": "0199...", "address": "г. Казань, ул. Демонстрационная, д. 1"},
      "status": "delivered",
      "support_m2": "1240.00",
      "delivered_at": "2026-10-03T12:00:00+03:00",
      "uk_due_at": "2026-11-17T12:00:00+03:00",
      "overdue": false,
      "meeting_id": null
    }
  ]
}
```

**Ошибки:** `403 not_staff`, `404 org_not_found`.

## Собрание

Собрание ведёт администратор — сотрудник УК, который ведёт инициативу (решение 79). Остальные ручки ниже доступны
только ему, если не сказано иное.

### `POST /api/v1/initiatives/{id}/meetings`

**Каркас (Г2).** Сотрудник УК создаёт собрание по требованию, стадия инициативы — `demand`. В той же транзакции
создаются бюллетени по всем собственникам снимка реестра инициативы (вес копируется), а инициатива переходит на
стадию `meeting`.

**Request body:**

```json
{
  "form": "paper_absentee",
  "notice_at": "2026-10-05T12:00:00+03:00",
  "voting_starts_at": "2026-10-15T09:00:00+03:00",
  "voting_ends_at": "2026-10-25T20:00:00+03:00",
  "chair_owner_id": "0199...",
  "secretary_owner_id": "0199..."
}
```

- `form` — `gis_electronic` или `paper_absentee`.
- Даты: в обычном доме голосование начинается не раньше чем через 10 дней после `notice_at` (ст. 45 ч. 4 ЖК). В
  демо-доме подходят любые будущие даты.
- Председатель и секретарь — разные собственники снимка. Кандидатов отдаёт
  `GET /houses/{houseID}/meeting-officer-candidates`.

**Response `201 Created`:** как `GET /meetings/{id}`.

**Ошибки:**
- `403 staff_only`;
- `404 initiative_not_found`;
- `409 wrong_stage`, `409 active_meeting_exists`;
- `400 invalid_dates`, `400 invalid_officers`.

### `GET /api/v1/meetings/{id}`

**Каркас (Г3).** Сроки, статус, повестка и прогресс собрания. Видят администратор и подтверждённые жители дома.

**Response `200 OK`:**

```json
{
  "id": "0199...",
  "initiative_id": "0199...",
  "title": "Камеры в подъездах",
  "house": {"id": "0199...", "address": "г. Казань, ул. Демонстрационная, д. 1"},
  "attempt": 1,
  "form": "paper_absentee",
  "status": "voting",
  "notice_at": "2026-10-05T12:00:00+03:00",
  "voting_starts_at": "2026-10-15T09:00:00+03:00",
  "voting_ends_at": "2026-10-25T20:00:00+03:00",
  "chair": {"owner_id": "0199...", "masked_name": "Иванов И. И."},
  "secretary": {"owner_id": "0199...", "masked_name": "Петрова А. В."},
  "agenda_items": [
    {"id": "0199...", "position": 1, "text": "Избрать председателя и секретаря…", "majority_rule": "majority_of_participants"}
  ],
  "progress": {
    "ballots_total": 64,
    "ballots_received": 12,
    "participants_m2": "610.50",
    "total_m2": "3000.00",
    "quorum_above_m2": "1500.00"
  },
  "is_admin": true,
  "outcome": null,
  "finalized_at": null
}
```

- `status` считается по датам: `preparation` → `notice` → `voting` → `counting` (голосование окончено) → `completed`.
- `is_admin` — может ли пользователь вести собрание.
- `outcome` после фиксации — `held` или `no_quorum`.

**Ошибки:** `403 not_member`, `404 meeting_not_found`.

### `GET /api/v1/meetings/{id}/tracker`

**Каркас (Г3).** Трекер администратора: список бюллетеней без вариантов голоса.

**Response `200 OK`:**

```json
{
  "summary": {"ballots_total": 64, "received": 12, "counted": 0, "participants_m2": "610.50"},
  "ballots": [
    {
      "id": "0199...",
      "premise_number": "45",
      "entrance": 3,
      "owner_masked_name": "Иванов И. И.",
      "weight_m2": "26.15",
      "status": "not_voted"
    }
  ]
}
```

`status` — `not_voted`, `paper_received`, `counted` или `invalid`.

**Ошибки:** `403 staff_only`.

### `POST /api/v1/meetings/{id}/ballots/receive`

**Каркас (Г3).** Отмечает, что бумажный бюллетень из трекера получен. Отметка по QR-токену появится вместе со
сканером и в объём до 30.09 не входит.

**Request body:** `{"ballot_id": "0199..."}`.

**Response `200 OK`:** `{ballot_id, status: "paper_received", received_at}`.

**Ошибки:** `403 staff_only`, `404 ballot_not_found`, `409 already_received`.

### `PUT /api/v1/ballots/{id}/decisions`

**Каркас (Г4).** После окончания голосования вносит решения из бумажного бюллетеня по всем вопросам повестки.

**Request body:**

```json
{
  "decisions": [
    {"agenda_item_id": "0199...", "choice": "for"}
  ]
}
```

`choice` — `for`, `against` или `abstain`.

**Response `200 OK`:** `{ballot_id, status: "counted", decisions}`.

**Ошибки:**
- `403 staff_only`;
- `400 invalid_request` — решения не по всем вопросам;
- `409 voting_not_finished`, `409 already_finalized`.

### `GET /api/v1/meetings/{id}/result-preview`

**Каркас (Г5).** Кворум и результат каждого вопроса до фиксации.

**Response `200 OK`:**

```json
{
  "participants_m2": "2400.00",
  "total_m2": "3000.00",
  "quorum_reached": true,
  "agenda_results": [
    {
      "agenda_item_id": "0199...",
      "position": 2,
      "text": "Установить видеонаблюдение…",
      "majority_rule": "two_thirds_of_all",
      "for_m2": "2040.00",
      "against_m2": "300.00",
      "abstain_m2": "60.00",
      "accepted": true
    }
  ]
}
```

**Ошибки:** `403 staff_only`.

### `POST /api/v1/meetings/{id}/finalize`

**Каркас (Г5).**
- Записывает итог один раз в `meeting_results`; менять его потом запрещает триггер.
- Собрание переходит в `completed`, инициатива — тоже.

**Response `200 OK`:** `{meeting_id, outcome, finalized_at, results}`, где `outcome` — `held` или `no_quorum`, а
`results` устроены как `agenda_results`.

**Ошибки:** `403 staff_only`, `409 voting_not_finished`, `409 already_finalized`.

### `GET /api/v1/meetings/{id}/protocol.pdf`

**Каркас (Д5).** Черновик протокола по Приказу Минстроя № 44/пр, строится только из зафиксированного итога.
Ответ — `application/pdf`.

**Ошибки:** `403 forbidden`, `409 not_finalized`.

### Демо-ускорители

**Каркас (Г6).** `POST /api/v1/meetings/{id}/demo/finish-voting` и `POST /api/v1/meetings/{id}/demo/fill-ballots`.
Работают только в демо-доме и только у администратора собрания. Без них жюри пришлось бы ждать дни и вносить
десятки бюллетеней вручную.
- `finish-voting` переносит окончание голосования на «сейчас»: можно вносить решения и фиксировать итог.
- `fill-ballots` отмечает полученными и учтёнными бюллетени примерно на 80% площади, «за» — около 85% участников,
  чтобы камеры набрали 2/3 от всех.

**Response `200 OK`:** как `GET /meetings/{id}`.

**Ошибки:** `403 not_demo`, `403 staff_only`.

## Вне объёма до 30.09

Ниже описан дизайн на будущее, эти ручки в «Известных ограничениях» README. Внесение итогов ГИС ЖКХ — пункт Should
(Г8), если останется время.

### Путь B — самостоятельная организация без УК

Путь B не создаёт `Demand`: инициатор берёт организацию собрания на себя, после чего вызывает
`POST /api/v1/initiatives/{id}/meetings`.

```text
предварительный опрос завершён
└─ POST /initiatives/{id}/self-organize
   └─ Initiative.path = B
      └─ выбор председателя и секретаря
         └─ POST /initiatives/{id}/meetings
            ├─ публикация уведомления о собрании
            ├─ собственники голосуют онлайн в ГИС ЖКХ
            ├─ инициатор переносит агрегаты через PUT /meetings/{id}/gis-results
            └─ POST /meetings/{id}/finalize → официальный итог
```

На пути B инициатор становится администратором собрания. УК не создаёт собрание и не получает право
изменять его данные только на основании управления домом.

### `POST /api/v1/initiatives/{id}/self-organize`

Выбирает путь B. Ручку вызывает подтверждённый собственник — инициатор. Она доступна после завершения
предварительного опроса либо после просрочки требования на пути A, если активное собрание ещё не создано.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "initiative_id": "0199...",
  "stage": "poll",
  "path": "B",
  "allowed_actions": [
    {"code": "create_meeting", "allowed": true}
  ]
}
```

Операция идемпотентна. Если путь B уже выбран, сервер возвращает текущее состояние. Если УК уже
создала активное собрание, возвращается `409 active_meeting_exists`.

### `PUT /api/v1/meetings/{id}/my-online-status`

Позволяет собственнику отметить, что он уже проголосовал онлайн в ГИС ЖКХ. Это статус «со слов» для
трекера и отключения напоминаний, а не официальный голос. Конкретные решения пользователя по вопросам
повестки backend не принимает и не возвращает.

Если у пользователя несколько объектов собственности, он явно передаёт те записи собственников,
по которым проголосовал.

**Request body:**

```json
{
  "owner_ids": ["0199..."],
  "voted": true
}
```

**Response `200 OK`:**

```json
{
  "meeting_id": "0199...",
  "ballots": [
    {
      "owner_id": "0199...",
      "status": "online_declared",
      "declared_at": "2026-10-20T15:30:00+03:00"
    }
  ]
}
```

`voted: false` снимает отметку до окончания голосования. После окончания собрания ручка возвращает
`409 voting_finished`. Официальные результаты онлайн-голосования появляются только через
`PUT /api/v1/meetings/{id}/gis-results`.

### `GET /api/v1/meetings/{id}/my-ballots`

Возвращает бумажные бюллетени текущего пользователя, по одному на каждого доступного ему собственника.
Для собрания только в форме `gis_electronic` возвращает пустой массив.

**Request body:** отсутствует.

**Response `200 OK`:**

```json
{
  "ballots": [
    {
      "id": "0199...",
      "owner_id": "0199...",
      "premise": {"id": "0199...", "number": "45"},
      "status": "not_voted",
      "download_url": "/api/v1/ballots/0199.../pdf"
    }
  ]
}
```

### `GET /api/v1/ballots/{id}/pdf`

Генерирует один бумажный бюллетень. Доступ разрешён только пользователю с подтверждённой привязкой к
собственнику этого бюллетеня либо администратору собрания.

**Request body:** отсутствует.

**Response `200 OK`:** PDF, `Content-Type: application/pdf`.

### `PUT /api/v1/meetings/{id}/gis-results`

Вносит официальные агрегированные результаты онлайн-голосования из ГИС ЖКХ. В MVP данные модельные.

**Request body:**

```json
{
  "entries": [
    {
      "agenda_item_id": "0199...",
      "for_m2": "1800.00",
      "against_m2": "100.00",
      "abstain_m2": "50.00"
    }
  ],
  "online_participants_m2": "1950.00"
}
```

**Response `200 OK`:** сохранённые официальные агрегаты.

### `GET /api/v1/meetings/{id}/notice.pdf`

Генерирует сообщение о проведении собрания.

**Request body:** отсутствует.

**Response `200 OK`:** PDF, `Content-Type: application/pdf`.

### `GET /api/v1/meetings/{id}/ballots.zip`

Генерирует персональные бюллетени с QR-кодами.

**Request body:** отсутствует.

**Response `200 OK`:** ZIP-архив с PDF-бюллетенями.

## Минимум для подключения реальной УК после демо

Эти маршруты можно не делать для первого сквозного демо с сидированным домом, но они нужны для
подключения настоящей управляющей организации.

### `POST /api/v1/orgs/{orgID}/houses`

Подключает новый дом к УК.

**Request body:** `{address, fias_id, region, timezone, passport_area_m2}`.

**Response `201 Created`:** `{id, org_id, address, invite_slug}`.

### `POST /api/v1/houses/{houseID}/registry-uploads`

Загружает CSV реестра и создаёт версию в статусе предпросмотра.

**Request body:** `multipart/form-data` с CSV-файлом.

**Response `201 Created`:** `{id, version, status: "preview", matching_report}`.

### `GET /api/v1/registry-uploads/{id}`

Возвращает отчёт сопоставления собственников, привязок и площадей до применения версии.

**Request body:** отсутствует.

**Response `200 OK`:** `{id, version, status, total_area_m2, passport_difference_m2, matching_report}`.

### `POST /api/v1/registry-uploads/{id}/apply`

Применяет проверенную версию реестра и делает её текущей.

**Request body:** отсутствует.

**Response `200 OK`:** `{id, status: "applied", applied_at}`.

### `POST /api/v1/memberships/{id}/verify/account`

Подтверждает роль жителя по лицевому счёту и сумме последнего начисления.

**Request body:** `{account: "...", last_charge: "1250.45"}`.

**Response `200 OK`:** `{id, role: "resident", status: "verified", method: "account"}`.

### `POST /api/v1/memberships/{id}/request-owner-verification`

Создаёт заявку на ручное подтверждение собственника сотрудником УК.

**Request body:** `{owner_id: "0199..."}`.

**Response `202 Accepted`:** `{membership_id, status: "pending", method: "uk_manual"}`.

### `POST /api/v1/memberships/{id}/data-correction-requests`

Создаёт обращение «Данные неверны» по привязке пользователя. Backend сам фиксирует пользователя,
помещение, дом и текущую версию реестра, поэтому подменить их в теле запроса нельзя.

**Request body:**

```json
{
  "reason": "wrong_share",
  "comment": "В реестре указана доля 1/2, должна быть 1/1"
}
```

`reason` принимает `wrong_name`, `wrong_share`, `wrong_area`, `wrong_premise` или `other`.

**Response `201 Created`:**

```json
{
  "id": "0199...",
  "membership_id": "0199...",
  "status": "pending",
  "created_at": "2026-10-01T12:00:00+03:00"
}
```

Таблица `registry_correction_requests` уже предусмотрена миграциями; HTTP-обработчик и список таких
обращений в кабинете УК ещё нужно реализовать.

### `GET /api/v1/orgs/{orgID}/verification-requests`

Возвращает сотруднику УК ожидающие заявки на подтверждение собственников.

**Request body:** отсутствует.

**Response `200 OK`:** список заявок с помещением и замаскированным ФИО собственника.

### `POST /api/v1/memberships/{id}/approve`

Подтверждает заявку собственника.

**Request body:** отсутствует.

**Response `200 OK`:** `{id, role: "owner", status: "verified", method: "uk_manual"}`.

### `POST /api/v1/memberships/{id}/reject`

Отклоняет заявку собственника с обязательной причиной.

**Request body:** `{reason: "Данные не совпадают с реестром"}`.

**Response `200 OK`:** `{id, status: "rejected", reason}`.

Подтверждение по телефону не требует отдельной HTTP-ручки мини-приложения: подтверждённый номер
приходит событием `requestContact` от MAX-бота, после чего сервер сравнивает его HMAC с реестром.
