package rules

// Decision types and templates of the MVP (docs/04, решения 7, 9, 13): platform data,
// seeded on every start. A change of a template after launch is a new version: the
// initiatives keep the version they were created with. Before launch v1 is edited in
// place.

// TemplateVideoSurveillance is the code of the «Видеонаблюдение» template (docs/API_DESCRIPTION.md).
const TemplateVideoSurveillance = "video_surveillance"

type decisionTypeDef struct {
	Code, Name, Rule, LegalReference string
}

var decisionTypes = []decisionTypeDef{
	// Пользование общим имуществом (п. 3 ч. 2 ст. 44 ЖК) решается большинством не менее
	// 2/3 голосов от общего числа голосов собственников (ч. 1 ст. 46 ЖК). Видеонаблюдение
	// в подъезде попадает сюда (позиция ВС РФ, docs/00). Часть 2 статьи 46 — о другом:
	// собрание не решает вопросы вне повестки.
	{
		Code:           "common_property_use",
		Name:           "Пользование общим имуществом (в том числе видеонаблюдение)",
		Rule:           string(TwoThirdsOfAll),
		LegalReference: "ЖК РФ, ч. 1 ст. 46, п. 3 ч. 2 ст. 44",
	},
	// Ст. 46 ч. 1 ЖК: общее правило для остальных решений собрания.
	{
		Code:           "routine",
		Name:           "Обычные решения общего собрания",
		Rule:           string(MajorityOfParticipants),
		LegalReference: "ЖК РФ, ст. 46 ч. 1",
	},
}

// templateDef is a template as the platform writes it. ParamsSchema is the form of the
// template (params.go describes the subset); UISchema only tells the mini-app how to
// show it: the order of the fields, widgets and the titles of enum values.
type templateDef struct {
	Code         string
	Version      int
	Name         string
	Description  string
	ParamsSchema string
	UISchema     string
	Items        []CatalogItem
}

var templates = []templateDef{videoSurveillance}

// videoSurveillance is the «Видеонаблюдение» template (docs/02, шаг 1): where the
// cameras go, how many, the estimated cost, how it is paid and who sees the records.
// The values go to the materials of the meeting; money is not modelled (решение 52).
//
// The first agenda item is procedural (решение 74): the protocol names the chair and
// the secretary who counted the votes (Приказ Минстроя № 44/пр), and the meeting
// itself elects them by the majority of participants.
var videoSurveillance = templateDef{
	Code:        TemplateVideoSurveillance,
	Version:     1,
	Name:        "Видеонаблюдение",
	Description: "Камеры в подъездах: где ставим, кто хранит записи, как оплачиваем.",
	ParamsSchema: `{
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
			"camera_count": {
				"type": "integer",
				"title": "Количество камер",
				"minimum": 1,
				"maximum": 1000
			},
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
	}`,
	UISchema: `{
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
	}`,
	Items: []CatalogItem{
		{
			Position:     1,
			Text:         "Избрать председателя и секретаря общего собрания и наделить их полномочиями по подсчёту голосов",
			DecisionCode: "routine",
		},
		{
			Position:     2,
			Text:         "Установить видеонаблюдение в подъездах дома (монтаж, хранение записей и оплата — по проекту, приложенному к материалам собрания)",
			DecisionCode: "common_property_use",
		},
	},
}
