// Generated from screens/Create.aui; edit the AUI source.
import { Alert, Badge, Button, Card, Checkbox, Divider, Field, Heading, Input, Page, Row, Select, Stack, Text } from "@/components/ui"

export function Create({ data, onAction }: { data: any; onAction: (name: string) => void }) {
  return (
    <Page>
      <Row variant="page-header">
        <Stack gap="sm">
          <Text variant="eyebrow">NEW PROJECT</Text>
          <Heading level={1}>Give your next idea a home</Heading>
          <Text variant="muted">Start with a repository or upload your files after creating the project.</Text>
        </Stack>
        <Button variant="secondary" onClick={() => onAction("home")}>Cancel</Button>
      </Row>
      <Row variant="steps">
        <Badge tone={data.sourceTone}>1 · Source</Badge>
        <Divider />
        <Badge tone={data.buildTone}>2 · Build</Badge>
        <Divider />
        <Badge tone={data.reviewTone}>3 · Review</Badge>
      </Row>
      <Card variant="form">
        {data.createSource && (
          <>
            <Heading level={2}>Project and source</Heading>
            <Field label="Project name"><Input name="name" placeholder="My lovely website" required={true} /></Field>
            <Field label="Site name"><Input name="slug" placeholder="my-lovely-website" required={true} /></Field>
            <Text variant="caption">Lowercase letters, numbers and hyphens. This forms your permanent local site address.</Text>
            <Field label="GitHub repository (optional)"><Input name="repository" placeholder="owner/repository" /></Field>
            <Field label="Production branch"><Input name="branch" placeholder="main" /></Field>
            <Text variant="muted">Private repositories can be connected with a protected, fine-grained GitHub credential after creation. For ZIP uploads, leave the repository empty.</Text>
            <Button onClick={() => onAction("createNext")}>Continue to build settings →</Button>
          </>
        )}
        {data.createBuild && (
          <>
            <Heading level={2}>How should this project run?</Heading>
            <Field label="Deployment adapter"><Select name="adapter" options={data.adapterOptions} /></Field>
            <Field label="Node runtime (only installed when needed)"><Select name="nodeMajor" options={data.nodeOptions} /></Field>
            <Field label="Install command · JSON argument array"><Input name="install" placeholder="JSON arguments, for example npm ci" /></Field>
            <Field label="Build command · JSON argument array"><Input name="build" placeholder="JSON arguments for your build script" /></Field>
            <Field label="Static output directory"><Input name="output" placeholder="dist or . for an uploaded site root" /></Field>
            <Field label="Start command · generic Node adapter"><Input name="start" placeholder="JSON arguments for the Node server" /></Field>
            <Text variant="caption">Next.js uses standalone output; Nuxt uses Nitro node-server output. Empty install/build commands suit already-built static ZIPs.</Text>
            <Row>
              <Button variant="secondary" onClick={() => onAction("createBack")}>Back</Button>
              <Button onClick={() => onAction("createNext")}>Review project →</Button>
            </Row>
          </>
        )}
        {data.createReview && (
          <>
            <Heading level={2}>Ready for its first deployment</Heading>
            <Text>{data.createSummary}</Text>
            <Field label="Preview deployments"><Checkbox name="previews" label="Allow preview branches and releases" /></Field>
            <Field label="Source approval"><Checkbox name="trusted" label="I administer and trust this project's source and build scripts." /></Field>
            <Alert tone="info">Static files need Caddy. Executable builds and Node apps also need a verified Node runtime. Dependencies download on demand; Settings supports verified offline archives.</Alert>
            <Row>
              <Button variant="secondary" onClick={() => onAction("createBack")}>Back</Button>
              <Button onClick={() => onAction("saveNewProject")}>Create project</Button>
            </Row>
          </>
        )}
      </Card>
    </Page>
  )
}
