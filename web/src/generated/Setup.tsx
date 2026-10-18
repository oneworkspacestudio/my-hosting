// Generated from screens/Setup.aui; edit the AUI source.
import { Alert, Button, Card, Field, Heading, Image, Input, Page, Text } from "@/components/ui"

export function Setup({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Card variant="login">
        <Image src={data.logo} width={76} height={76} alt="My Hosting" />
        <Text variant="eyebrow">WELCOME HOME</Text>
        <Heading level={1}>Your hosting starts here.</Heading>
        <Text variant="muted">Create the standalone administrator. This private setup link works once, on this installation.</Text>
        {data.error && (
          <Alert tone="error">{data.error}</Alert>
        )}
        <Field label="Email address"><Input name="setupEmail" type="email" autoComplete="username" /></Field>
        <Field label="Password · at least 8 characters"><Input name="setupPassword" type="password" autoComplete="new-password" /></Field>
        <Field label="Confirm password"><Input name="setupConfirm" type="password" autoComplete="new-password" /></Field>
        <Button onClick={() => onAction("bootstrap")}>Create administrator & open Hosting</Button>
        <Text variant="caption">Workspace installations connect through MyAccounts automatically and do not create another password.</Text>
      </Card>
    </Page>
  )
}
