-- Document hard mode in the /options catalog.
UPDATE option_help SET description =
  'List same item+amount+date entries with one-import/entered-separately hints (you decide what to delete). Add "hard" to match amount+date regardless of item name — catches text vs photo double-posts.'
WHERE command = 'check duplicate <month> / cek duplikat <bulan>';
