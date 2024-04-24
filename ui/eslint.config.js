let eslint = require('@eslint/js');
let tselint = require('typescript-eslint');

module.exports = [
  eslint.configs.recommended,
  ...tselint.configs.recommended,
  {
    files: ['**/*.js', '**/*.cjs', '**/*.mjs', '**/*.ts', '**/*.tsx'],
  },
];
