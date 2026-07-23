export type PhoneCountry = {
  code: string;
  dial: string;
  name: string;
};

const RAW: { code: string; dial: string }[] = [
  { code: "AF", dial: "93" },
  { code: "AL", dial: "355" },
  { code: "DZ", dial: "213" },
  { code: "AD", dial: "376" },
  { code: "AO", dial: "244" },
  { code: "AR", dial: "54" },
  { code: "AM", dial: "374" },
  { code: "AU", dial: "61" },
  { code: "AT", dial: "43" },
  { code: "AZ", dial: "994" },
  { code: "BH", dial: "973" },
  { code: "BD", dial: "880" },
  { code: "BY", dial: "375" },
  { code: "BE", dial: "32" },
  { code: "BZ", dial: "501" },
  { code: "BJ", dial: "229" },
  { code: "BT", dial: "975" },
  { code: "BO", dial: "591" },
  { code: "BA", dial: "387" },
  { code: "BW", dial: "267" },
  { code: "BR", dial: "55" },
  { code: "BN", dial: "673" },
  { code: "BG", dial: "359" },
  { code: "BF", dial: "226" },
  { code: "BI", dial: "257" },
  { code: "KH", dial: "855" },
  { code: "CM", dial: "237" },
  { code: "CA", dial: "1" },
  { code: "CV", dial: "238" },
  { code: "CF", dial: "236" },
  { code: "TD", dial: "235" },
  { code: "CL", dial: "56" },
  { code: "CN", dial: "86" },
  { code: "CO", dial: "57" },
  { code: "KM", dial: "269" },
  { code: "CG", dial: "242" },
  { code: "CD", dial: "243" },
  { code: "CR", dial: "506" },
  { code: "HR", dial: "385" },
  { code: "CU", dial: "53" },
  { code: "CY", dial: "357" },
  { code: "CZ", dial: "420" },
  { code: "DK", dial: "45" },
  { code: "DJ", dial: "253" },
  { code: "DM", dial: "1767" },
  { code: "DO", dial: "1809" },
  { code: "EC", dial: "593" },
  { code: "EG", dial: "20" },
  { code: "SV", dial: "503" },
  { code: "GQ", dial: "240" },
  { code: "ER", dial: "291" },
  { code: "EE", dial: "372" },
  { code: "ET", dial: "251" },
  { code: "FJ", dial: "679" },
  { code: "FI", dial: "358" },
  { code: "FR", dial: "33" },
  { code: "GA", dial: "241" },
  { code: "GM", dial: "220" },
  { code: "GE", dial: "995" },
  { code: "DE", dial: "49" },
  { code: "GH", dial: "233" },
  { code: "GR", dial: "30" },
  { code: "GT", dial: "502" },
  { code: "GN", dial: "224" },
  { code: "GW", dial: "245" },
  { code: "GY", dial: "592" },
  { code: "HT", dial: "509" },
  { code: "HN", dial: "504" },
  { code: "HK", dial: "852" },
  { code: "HU", dial: "36" },
  { code: "IS", dial: "354" },
  { code: "IN", dial: "91" },
  { code: "ID", dial: "62" },
  { code: "IR", dial: "98" },
  { code: "IQ", dial: "964" },
  { code: "IE", dial: "353" },
  { code: "IL", dial: "972" },
  { code: "IT", dial: "39" },
  { code: "JM", dial: "1876" },
  { code: "JP", dial: "81" },
  { code: "JO", dial: "962" },
  { code: "KZ", dial: "7" },
  { code: "KE", dial: "254" },
  { code: "KI", dial: "686" },
  { code: "KP", dial: "850" },
  { code: "KR", dial: "82" },
  { code: "KW", dial: "965" },
  { code: "KG", dial: "996" },
  { code: "LA", dial: "856" },
  { code: "LV", dial: "371" },
  { code: "LB", dial: "961" },
  { code: "LS", dial: "266" },
  { code: "LR", dial: "231" },
  { code: "LY", dial: "218" },
  { code: "LI", dial: "423" },
  { code: "LT", dial: "370" },
  { code: "LU", dial: "352" },
  { code: "MO", dial: "853" },
  { code: "MK", dial: "389" },
  { code: "MG", dial: "261" },
  { code: "MW", dial: "265" },
  { code: "MY", dial: "60" },
  { code: "MV", dial: "960" },
  { code: "ML", dial: "223" },
  { code: "MT", dial: "356" },
  { code: "MR", dial: "222" },
  { code: "MU", dial: "230" },
  { code: "MX", dial: "52" },
  { code: "MD", dial: "373" },
  { code: "MC", dial: "377" },
  { code: "MN", dial: "976" },
  { code: "ME", dial: "382" },
  { code: "MA", dial: "212" },
  { code: "MZ", dial: "258" },
  { code: "MM", dial: "95" },
  { code: "NA", dial: "264" },
  { code: "NP", dial: "977" },
  { code: "NL", dial: "31" },
  { code: "NZ", dial: "64" },
  { code: "NI", dial: "505" },
  { code: "NE", dial: "227" },
  { code: "NG", dial: "234" },
  { code: "NO", dial: "47" },
  { code: "OM", dial: "968" },
  { code: "PK", dial: "92" },
  { code: "PS", dial: "970" },
  { code: "PA", dial: "507" },
  { code: "PG", dial: "675" },
  { code: "PY", dial: "595" },
  { code: "PE", dial: "51" },
  { code: "PH", dial: "63" },
  { code: "PL", dial: "48" },
  { code: "PT", dial: "351" },
  { code: "QA", dial: "974" },
  { code: "RO", dial: "40" },
  { code: "RU", dial: "7" },
  { code: "RW", dial: "250" },
  { code: "SA", dial: "966" },
  { code: "SN", dial: "221" },
  { code: "RS", dial: "381" },
  { code: "SC", dial: "248" },
  { code: "SL", dial: "232" },
  { code: "SG", dial: "65" },
  { code: "SK", dial: "421" },
  { code: "SI", dial: "386" },
  { code: "SO", dial: "252" },
  { code: "ZA", dial: "27" },
  { code: "SS", dial: "211" },
  { code: "ES", dial: "34" },
  { code: "LK", dial: "94" },
  { code: "SD", dial: "249" },
  { code: "SR", dial: "597" },
  { code: "SZ", dial: "268" },
  { code: "SE", dial: "46" },
  { code: "CH", dial: "41" },
  { code: "SY", dial: "963" },
  { code: "TW", dial: "886" },
  { code: "TJ", dial: "992" },
  { code: "TZ", dial: "255" },
  { code: "TH", dial: "66" },
  { code: "TL", dial: "670" },
  { code: "TG", dial: "228" },
  { code: "TO", dial: "676" },
  { code: "TT", dial: "1868" },
  { code: "TN", dial: "216" },
  { code: "TR", dial: "90" },
  { code: "TM", dial: "993" },
  { code: "UG", dial: "256" },
  { code: "UA", dial: "380" },
  { code: "AE", dial: "971" },
  { code: "GB", dial: "44" },
  { code: "US", dial: "1" },
  { code: "UY", dial: "598" },
  { code: "UZ", dial: "998" },
  { code: "VU", dial: "678" },
  { code: "VE", dial: "58" },
  { code: "VN", dial: "84" },
  { code: "YE", dial: "967" },
  { code: "ZM", dial: "260" },
  { code: "ZW", dial: "263" },
];

function buildNames(locale: string): Map<string, string> {
  const names = new Map<string, string>();
  let displayNames: Intl.DisplayNames | null = null;
  try {
    displayNames = new Intl.DisplayNames([locale], { type: "region" });
  } catch {
    displayNames = null;
  }
  for (const c of RAW) {
    let name = c.code;
    if (displayNames) {
      const resolved = displayNames.of(c.code);
      if (resolved) name = resolved;
    }
    names.set(c.code, name);
  }
  return names;
}

const NAMES_BY_LOCALE = new Map<string, Map<string, string>>();

export function getPhoneCountries(locale = "en"): PhoneCountry[] {
  let names = NAMES_BY_LOCALE.get(locale);
  if (!names) {
    names = buildNames(locale);
    NAMES_BY_LOCALE.set(locale, names);
  }
  return RAW.map((c) => ({
    code: c.code,
    dial: c.dial,
    name: names.get(c.code) ?? c.code,
  }));
}

export function findCountryByCode(code: string, locale = "en"): PhoneCountry | undefined {
  return getPhoneCountries(locale).find((c) => c.code === code);
}

export function findCountryByDial(dial: string, locale = "en"): PhoneCountry | undefined {
  if (!dial) return undefined;
  const all = getPhoneCountries(locale);
  let best: PhoneCountry | undefined;
  for (const c of all) {
    if (dial.startsWith(c.dial) && (!best || c.dial.length > best.dial.length)) {
      best = c;
    }
  }
  return best;
}

export function flagEmoji(code: string): string {
  return code.toUpperCase().replace(/./g, (c) => String.fromCodePoint(127397 + c.charCodeAt(0)));
}
