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
  }
}
```

Если приложение открыто без ссылки дома, поле `house` равно `null`. При неверном или устаревшем
`initData` возвращается `401 Unauthorized`.

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

---

## Минимальный API сквозного MVP

Маршруты ниже ещё не реализованы. Они составляют минимальный контракт для сценария
«привязка к дому → инициатива → опрос → требование в УК → собрание → итог».

В документах идея жителя называется `Initiative`, поэтому API использует `/initiatives`, а не
`/issues`. Отдельная заявка в УК с фотографией (`IssueReport`) находится вне текущего MVP.

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

### `GET /api/v1/templates`

Возвращает доступные шаблоны инициатив, минимум — «Видеонаблюдение».

**Request body:** отсутствует.

**Response `200 OK`:** список `{code, name, version}`.

### `GET /api/v1/templates/{code}`

Возвращает поля формы шаблона, вопросы повестки и правила большинства.

**Request body:** отсутствует.

**Response `200 OK`:** `{code, name, version, params_schema, agenda_items}`.

### `GET /api/v1/houses/{houseID}/initiatives`

Возвращает инициативы дома, доступные текущему подтверждённому пользователю.

**Request body:** отсутствует.

**Response `200 OK`:** список `{id, title, stage, path, poll_ends_at, created_at}`.

### `POST /api/v1/houses/{houseID}/initiatives`

Создаёт инициативу из шаблона и фиксирует текущую версию реестра. Житель создаёт черновик,
подтверждённый собственник может стать инициатором.

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

**Response `201 Created`:** `{id, house_id, stage: "draft", registry_version, agenda_items}`.

### `GET /api/v1/initiatives/{id}`

Возвращает карточку инициативы, повестку, стадию, пороги и разрешённые текущему пользователю действия.

**Request body:** отсутствует.

**Response `200 OK`:** `{id, title, description, stage, path, agenda_items, thresholds, allowed_actions}`.

### `POST /api/v1/initiatives/{id}/start-poll`

Запускает предварительный опрос и создаёт задачи рассылки подтверждённым собственникам.

**Request body:**

```json
{
  "ends_at": "2026-10-02T18:00:00+03:00"
}
```

**Response `200 OK`:** `{id, stage: "poll", poll_ends_at}`.

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

**Response `200 OK`:** `{choice, weight_m2, updated_at}`.

### `GET /api/v1/initiatives/{id}/poll`

Возвращает прогресс предварительного опроса в м² без раскрытия чужих вариантов голосования.

**Request body:** отсутствует.

**Response `200 OK`:** `{for_m2, against_m2, total_m2, demand_m2, demand_reached, poll_ends_at}`.

### `POST /api/v1/initiatives/{id}/demand`

При поддержке не менее 10% выбирает путь A и создаёт требование к УК провести собрание.

**Request body:**

```json
{
  "channel": "paper"
}
```

`channel` принимает `paper` или `gosuslugi_dom`.

**Response `201 Created`:** `{id, initiative_id, channel, support_m2, status: "draft"}`.

### `POST /api/v1/demands/{id}/mark-delivered`

Отмечает передачу требования в УК и запускает отсчёт 45 дней.

**Request body:**

```json
{
  "delivered_at": "2026-10-03T12:00:00+03:00"
}
```

**Response `200 OK`:** `{id, delivered_at, uk_due_at, overdue: false}`.

### `GET /api/v1/demands/{id}/pdf`

Генерирует требование в УК по текущим данным.

**Request body:** отсутствует.

**Response `200 OK`:** PDF, `Content-Type: application/pdf`.

## Минимальный API кабинета УК

### `GET /api/v1/orgs`

Возвращает управляющие организации, в которых текущий пользователь является сотрудником.

**Request body:** отсутствует.

**Response `200 OK`:** список `{id, type, name, role}`.

### `GET /api/v1/orgs/{orgID}/houses`

Возвращает дома выбранной УК.

**Request body:** отсутствует.

**Response `200 OK`:** список `{id, address, region, is_demo, current_registry_version}`.

### `GET /api/v1/orgs/{orgID}/demands`

Возвращает входящие требования собственников провести собрание.

**Request body:** отсутствует.

**Response `200 OK`:** список `{id, initiative_id, house, delivered_at, uk_due_at, overdue}`.

### `POST /api/v1/initiatives/{id}/meetings`

Создаёт собрание по требованию. На пути A вызывает сотрудник УК, на пути B — инициатор.

**Request body:**

```json
{
  "form": "gis_electronic",
  "notice_at": "2026-10-05T12:00:00+03:00",
  "voting_starts_at": "2026-10-15T09:00:00+03:00",
  "voting_ends_at": "2026-10-25T20:00:00+03:00",
  "chair_owner_id": "0199...",
  "secretary_owner_id": "0199..."
}
```

**Response `201 Created`:** `{id, initiative_id, attempt, status: "preparation", form}`.

### `GET /api/v1/meetings/{id}`

Возвращает сроки, статус, повестку и агрегированный прогресс собрания.

**Request body:** отсутствует.

**Response `200 OK`:** `{id, status, form, dates, agenda_items, participants_m2, quorum}`.

### `GET /api/v1/meetings/{id}/tracker`

Возвращает трекер «голосовал / не голосовал». Доступные детали зависят от роли пользователя.

**Request body:** отсутствует.

**Response `200 OK`:** `{summary, premises}` без раскрытия конкретного варианта чужого голоса.

### `POST /api/v1/meetings/{id}/ballots/receive`

Отмечает получение бумажного бюллетеня по QR-токену или номеру помещения.

**Request body:**

```json
{
  "qr_token": "url-safe-token"
}
```

**Response `200 OK`:** `{ballot_id, status: "paper_received", received_at}`.

### `PUT /api/v1/ballots/{id}/decisions`

После окончания голосования вносит решения из бумажного бюллетеня.

**Request body:**

```json
{
  "decisions": [
    {"agenda_item_id": "0199...", "choice": "for"}
  ]
}
```

`choice` принимает `for`, `against` или `abstain`.

**Response `200 OK`:** `{ballot_id, status: "counted", decisions}`.

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

### `GET /api/v1/meetings/{id}/result-preview`

Считает кворум и результат каждого вопроса без окончательной фиксации.

**Request body:** отсутствует.

**Response `200 OK`:** `{quorum_reached, participants_m2, agenda_results}`.

### `POST /api/v1/meetings/{id}/finalize`

Неизменяемо фиксирует официальный результат собрания по каждому вопросу.

**Request body:** отсутствует.

**Response `200 OK`:** `{meeting_id, outcome, finalized_at, results}`.

### `GET /api/v1/meetings/{id}/notice.pdf`

Генерирует сообщение о проведении собрания.

**Request body:** отсутствует.

**Response `200 OK`:** PDF, `Content-Type: application/pdf`.

### `GET /api/v1/meetings/{id}/ballots.zip`

Генерирует персональные бюллетени с QR-кодами.

**Request body:** отсутствует.

**Response `200 OK`:** ZIP-архив с PDF-бюллетенями.

### `GET /api/v1/meetings/{id}/protocol.pdf`

Генерирует протокол только из зафиксированных `MeetingResult`.

**Request body:** отсутствует.

**Response `200 OK`:** PDF, `Content-Type: application/pdf`.

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
