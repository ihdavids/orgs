package orgc

/*
func ShowAllTodos(c *rpc.Client) {
	var reply common.Todos
	var q common.Query = common.Query{
		IsProject: false,
		Status:    []string{"TODO"},
	}
	err := c.Call("Db.QueryTodos", q, &reply)
	if err != nil {
		log.Printf("%v", err)
	} else {
		for _, v := range reply {
			log.Printf("%v", v.Headline)
		}
	}
}
*/
