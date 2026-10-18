import { transform } from 'oxc-transform';

// Nitro's default esbuild service uses child stdio pipes which Windows refuses
// inside a restricting-SID token. Use an in-process native transformer instead;
// keep the same restricted worker and Node server rather than relaxing access.
export default {
  ssr: true,
  telemetry: false,
  devtools: { enabled: false },
  nitro: {
    preset: 'node-server',
    hooks: {
      'rollup:before'(_nitro, config) {
        if (process.platform !== 'win32') return;
        const plugins = config.plugins as Array<{ name?: string }>;
        const index = plugins.findIndex(plugin => plugin?.name === 'esbuild');
        if (index < 0) throw new Error('Expected Nitro esbuild plugin is missing; review the adapter');
        plugins[index] = {
          name: 'myhosting-native-transform',
          async transform(code: string, id: string) {
            if (id.includes('node_modules') || !/\.(?:[cm]?js|[cm]?ts|jsx|tsx)$/.test(id)) return null;
            const result = await transform(id, code, { target: 'node24', sourcemap: true });
            if (result.errors.length) throw new Error(result.errors.map(error => error.message).join('\n'));
            return { code: result.code, map: result.map ?? null };
          },
        } as never;
      },
    },
  },
};
