// ESLint flat config for the web app. Core rules only, so no plugin needed.
//
// Hooks that need nuance to use correctly (dependency arrays to get right,
// or escape hatches out of React's data flow) are banned. Each message says
// what to write instead.
//
// Run:  npx eslint src
const BANNED_HOOKS = {
  useEffect: "Derive in render, move to the event handler, use a key, the data layer, or useSyncExternalStore.",
  useLayoutEffect: "Use a ref callback for DOM work; otherwise see the useEffect rewrites.",
  useInsertionEffect: "Only CSS-in-JS libraries need this.",
  useMemo: "Compute directly; React Compiler handles memoization.",
  useCallback: "Use a plain function; React Compiler handles memoization.",
  useImperativeHandle: "Lift state up and pass props down instead.",
};
const entries = Object.entries(BANNED_HOOKS);

export default [
  {
    files: ["src/**/*.{js,jsx}"],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: {
      "no-restricted-imports": ["error", {
        paths: entries.map(([hook, fix]) => ({
          name: "react", importNames: [hook], message: `${hook} is banned (needs nuance). ${fix}`,
        })),
      }],
      "no-restricted-properties": ["error", ...entries.map(([hook, fix]) => ({
        object: "React", property: hook, message: `${hook} is banned (needs nuance). ${fix}`,
      }))],
    },
  },
];
