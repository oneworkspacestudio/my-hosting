// Generated from screens/Login.aui; edit the AUI source.
import { Alert, Button, Card, Field, Heading, Image, Input, Page, Text } from "@/components/ui"

export function Login({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Card variant="login">
        <Image src={data.logo} width={76} height={76} alt="My Hosting" />
        <Text variant="eyebrow">ONE WORKSPACE</Text>
        <Heading level={1}>A home for your ideas.</Heading>
        <Text variant="muted">Deploy your sites and apps on your own server.</Text>
        {data.error && (
          <Alert tone="error">{data.error}</Alert>
        )}
        {data.oidc ? (
          <>
            <Button onClick={() => onAction("oidcLogin")}>Continue with MyAccounts</Button>
            <Text variant="caption">Already signed in to Workspace? Your existing MyAccounts session connects automatically.</Text>
          </>
        ) : (
          <>
            {data.bootstrapNeeded && (
              <Alert tone="info">Create the standalone owner locally before signing in: run MyHosting.exe --data YOUR_NTFS_FOLDER --email YOUR_EMAIL --password-stdin bootstrap. MyAccounts mode disables independent login.</Alert>
            )}
            <Field label="Email address"><Input name="loginEmail" type="email" autoComplete="username" /></Field>
            <Field label="Password"><Input name="loginPassword" type="password" autoComplete="current-password" /></Field>
            <Button onClick={() => onAction("localLogin")}>Sign in</Button>
          </>
        )}
        <Text variant="caption">Self hosted · Native Windows · No Docker</Text>
      </Card>
    </Page>
  )
}
