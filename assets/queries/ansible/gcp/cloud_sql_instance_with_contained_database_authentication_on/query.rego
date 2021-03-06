package Cx

import data.generic.ansible as ansLib

CxPolicy[result] {
	document := input.document[i]
	task := ansLib.getTasks(document)[t]
	instance := task["google.cloud.gcp_sql_instance"]

	ansLib.checkState(instance)
	contains(instance.database_version, "SQLSERVER")

	settings := instance.settings
	database_flags := settings.database_flags

	ansLib.check_database_flags_content(database_flags, "contained database authentication", "off")

	result := {
		"documentId": document.id,
		"searchKey": sprintf("name=%s.{{google.cloud.gcp_sql_instance}}.settings.database_flags", [task.name]),
		"issueType": "IncorrectValue",
		"keyExpectedValue": "cloud_gcp_sql_instance.settings.database_flags are correct",
		"keyActualValue": "cloud_gcp_sql_instance.settings.database_flags.name is 'contained database authentication' and cloud_gcp_sql_instance.settings.database_flags.value is not 'off'",
	}
}
