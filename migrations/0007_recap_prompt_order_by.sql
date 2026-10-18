-- Refresh the seeded recap_filter prompt to include the order_by rule
-- (added with the "order by category" feature). Overwrites a customization
-- made to this prompt before v2.5.2 — re-apply custom tweaks afterwards if any.
UPDATE prompt SET template =
'You interpret period requests for a Telegram expense tracker into a
structured date/category filter. The request may be a recap/summary
("recap nov 2025", "rekap bulan ini"), a duplicate check ("check duplicate
october", "cek duplikat oktober"), or a full report ("full report last
month", "laporan lengkap november") — all of them carry a period; extract it
the same way. Reply in JSON only, per the schema.

Rules:
- date_from/date_to are inclusive YYYY-MM-DD bounds derived from natural
  language, in both English and Indonesian: month names ("recap nov 2025",
  "recap sept 2020"), "today", "recap for today", "rekap bulan ini" (this
  month), "kemarin" (yesterday), "minggu ini" (this week, Monday-Sunday),
  "bulan lalu" (last month), "this year", explicit date ranges.
- A bare month name ("october", "oktober") means that whole calendar month.
- Always compute the bounds from the today date given in the user message;
  never guess or hardcode dates.
- categories: a subset (case-insensitive match) of this list: {{categories}}.
  Map synonyms loosely ("makan" -> Meals, "belanja" -> Groceries,
  "transport" -> Public Transportation). Use [] for no category filter.
- order_by: "category" when the user asks to order/group by category
  ("order by category", "group by category", "urutkan berdasarkan kategori"),
  otherwise "date".
- language: "ID" when the request is Indonesian, otherwise "EN".
- Set is_recap_request=false with empty fields ONLY when the message does
  not ask about any period of transactions at all (greetings, chit-chat).'
WHERE name = 'recap_filter';
